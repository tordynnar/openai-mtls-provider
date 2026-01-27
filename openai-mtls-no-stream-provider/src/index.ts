import type {
  LanguageModelV2,
  LanguageModelV2CallOptions,
  LanguageModelV2CallWarning,
  LanguageModelV2Content,
  LanguageModelV2FinishReason,
  LanguageModelV2FunctionTool,
  LanguageModelV2Message,
  LanguageModelV2StreamPart,
  LanguageModelV2ToolChoice,
  LanguageModelV2Usage,
} from '@ai-sdk/provider';
import * as fs from 'fs';
import * as path from 'path';

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

export interface MTLSProviderSettings {
  baseURL: string;
  name: string;
  apiKey?: string;
  headers?: Record<string, string>;
  queryParams?: Record<string, string>;
  clientCert?: string;
  clientKey?: string;
  caCert?: string;
  proxy?: string;
  removeKeys?: string[];
}

// ---------------------------------------------------------------------------
// Helpers (same as openai-mtls-provider)
// ---------------------------------------------------------------------------

function resolvePath(p: string): string {
  if (path.isAbsolute(p)) return p;
  return path.resolve(process.cwd(), p);
}

function removeKeysFromObject(obj: unknown, keysToRemove: string[]): unknown {
  if (obj === null || obj === undefined) return obj;
  if (Array.isArray(obj)) return obj.map(item => removeKeysFromObject(item, keysToRemove));
  if (typeof obj === 'object') {
    const result: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj)) {
      if (!keysToRemove.includes(key)) {
        result[key] = removeKeysFromObject(value, keysToRemove);
      }
    }
    return result;
  }
  return obj;
}

interface FetchOptions {
  clientCert?: string;
  clientKey?: string;
  caCert?: string;
  proxy?: string;
  removeKeys?: string[];
}

function createCustomFetch(options: FetchOptions): typeof fetch {
  let cert: Buffer | undefined;
  let key: Buffer | undefined;
  let ca: Buffer | undefined;

  if (options.clientCert && options.clientKey) {
    const certPath = resolvePath(options.clientCert);
    const keyPath = resolvePath(options.clientKey);
    try { cert = fs.readFileSync(certPath); } catch (e) {
      throw new Error(`Failed to read client certificate from ${certPath}: ${e}`);
    }
    try { key = fs.readFileSync(keyPath); } catch (e) {
      throw new Error(`Failed to read client key from ${keyPath}: ${e}`);
    }
    if (options.caCert) {
      const caPath = resolvePath(options.caCert);
      try { ca = fs.readFileSync(caPath); } catch (e) {
        throw new Error(`Failed to read CA certificate from ${caPath}: ${e}`);
      }
    }
  }

  return async (input: string | URL | Request, init?: RequestInit): Promise<Response> => {
    const fetchOptions: RequestInit & { tls?: object; proxy?: string } = { ...init };

    if (options.removeKeys && options.removeKeys.length > 0 && fetchOptions.body) {
      try {
        const bodyStr = typeof fetchOptions.body === 'string'
          ? fetchOptions.body
          : fetchOptions.body.toString();
        const bodyJson = JSON.parse(bodyStr);
        fetchOptions.body = JSON.stringify(removeKeysFromObject(bodyJson, options.removeKeys));
      } catch (_e) { /* not JSON */ }
    }

    if (cert && key) {
      // @ts-ignore – Bun supports tls option
      fetchOptions.tls = {
        cert: cert.toString(),
        key: key.toString(),
        ...(ca ? { ca: ca.toString(), rejectUnauthorized: true } : { rejectUnauthorized: false }),
      };
    }

    if (options.proxy) {
      // @ts-ignore – Bun supports proxy option
      fetchOptions.proxy = options.proxy;
    }

    return fetch(input, fetchOptions);
  };
}

// ---------------------------------------------------------------------------
// Message conversion: AI SDK V2 -> OpenAI chat format
// ---------------------------------------------------------------------------

interface OpenAIMessage {
  role: string;
  content?: string | Array<{ type: string; text?: string; image_url?: { url: string } }>;
  tool_calls?: Array<{
    id: string;
    type: 'function';
    function: { name: string; arguments: string };
  }>;
  tool_call_id?: string;
}

function convertMessages(messages: LanguageModelV2Message[]): OpenAIMessage[] {
  const result: OpenAIMessage[] = [];

  for (const msg of messages) {
    if (msg.role === 'system') {
      result.push({ role: 'system', content: msg.content });
      continue;
    }

    if (msg.role === 'user') {
      const parts: Array<{ type: string; text?: string; image_url?: { url: string } }> = [];
      for (const part of msg.content) {
        if (part.type === 'text') {
          parts.push({ type: 'text', text: part.text });
        } else if (part.type === 'file') {
          const mime = part.mediaType ?? 'application/octet-stream';
          if (typeof part.data === 'string') {
            parts.push({ type: 'image_url', image_url: { url: `data:${mime};base64,${part.data}` } });
          } else if (part.data instanceof URL) {
            parts.push({ type: 'image_url', image_url: { url: part.data.toString() } });
          }
        }
      }
      if (parts.length === 1 && parts[0].type === 'text') {
        result.push({ role: 'user', content: parts[0].text });
      } else {
        result.push({ role: 'user', content: parts });
      }
      continue;
    }

    if (msg.role === 'assistant') {
      let textContent = '';
      const toolCalls: OpenAIMessage['tool_calls'] = [];

      for (const part of msg.content) {
        if (part.type === 'text') {
          textContent += part.text;
        } else if (part.type === 'tool-call') {
          toolCalls.push({
            id: part.toolCallId,
            type: 'function',
            function: {
              name: part.toolName,
              arguments: typeof part.input === 'string' ? part.input : JSON.stringify(part.input),
            },
          });
        }
      }

      const assistantMsg: OpenAIMessage = { role: 'assistant' };
      if (textContent) assistantMsg.content = textContent;
      if (toolCalls.length > 0) assistantMsg.tool_calls = toolCalls;
      result.push(assistantMsg);
      continue;
    }

    if (msg.role === 'tool') {
      for (const part of msg.content) {
        if (part.type === 'tool-result') {
          let content: string;
          const output = part.output;
          if (output.type === 'text' || output.type === 'error-text') {
            content = output.value;
          } else if (output.type === 'json' || output.type === 'error-json') {
            content = JSON.stringify(output.value);
          } else if (output.type === 'content') {
            content = output.value
              .map((v: { type: string; text?: string }) =>
                v.type === 'text' ? v.text : '[media]',
              )
              .join('');
          } else {
            content = JSON.stringify(output);
          }
          result.push({
            role: 'tool',
            tool_call_id: part.toolCallId,
            content,
          });
        }
      }
      continue;
    }
  }

  return result;
}

function convertTools(
  tools: LanguageModelV2CallOptions['tools'],
): Array<{ type: 'function'; function: { name: string; description?: string; parameters: unknown } }> | undefined {
  if (!tools || tools.length === 0) return undefined;
  return tools
    .filter((t): t is LanguageModelV2FunctionTool => t.type === 'function')
    .map(t => ({
      type: 'function' as const,
      function: { name: t.name, description: t.description, parameters: t.inputSchema },
    }));
}

function convertToolChoice(
  tc: LanguageModelV2ToolChoice | undefined,
): string | { type: 'function'; function: { name: string } } | undefined {
  if (!tc) return undefined;
  switch (tc.type) {
    case 'auto': return 'auto';
    case 'none': return 'none';
    case 'required': return 'required';
    case 'tool': return { type: 'function', function: { name: tc.toolName } };
    default: return undefined;
  }
}

// ---------------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------------

function mapFinishReason(raw: string | undefined | null): LanguageModelV2FinishReason {
  switch (raw) {
    case 'stop': return 'stop';
    case 'length': return 'length';
    case 'content_filter': return 'content-filter';
    case 'tool_calls':
    case 'function_call': return 'tool-calls';
    default: return 'unknown';
  }
}

function parseUsage(usage: {
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
} | undefined): LanguageModelV2Usage {
  return {
    inputTokens: usage?.prompt_tokens ?? undefined,
    outputTokens: usage?.completion_tokens ?? undefined,
    totalTokens: usage?.total_tokens ?? undefined,
  };
}

function parseResponseContent(choice: {
  message?: {
    content?: string | null;
    tool_calls?: Array<{
      id: string;
      type: string;
      function: { name: string; arguments: string };
    }>;
  };
}): LanguageModelV2Content[] {
  const content: LanguageModelV2Content[] = [];
  const msg = choice.message;
  if (!msg) return content;

  if (msg.content) {
    content.push({ type: 'text', text: msg.content });
  }

  if (msg.tool_calls) {
    for (const tc of msg.tool_calls) {
      content.push({
        type: 'tool-call',
        toolCallId: tc.id,
        toolName: tc.function.name,
        input: tc.function.arguments,
      });
    }
  }

  return content;
}

// ---------------------------------------------------------------------------
// NoStreamChatLanguageModel (V2)
// ---------------------------------------------------------------------------

class NoStreamChatLanguageModel implements LanguageModelV2 {
  readonly specificationVersion = 'v2' as const;
  readonly provider: string;
  readonly modelId: string;
  readonly supportedUrls: Record<string, RegExp[]> = {};

  private baseURL: string;
  private apiKey?: string;
  private extraHeaders: Record<string, string>;
  private queryParams?: Record<string, string>;
  private customFetch: typeof fetch;

  constructor(
    modelId: string,
    provider: string,
    baseURL: string,
    options: {
      apiKey?: string;
      headers?: Record<string, string>;
      queryParams?: Record<string, string>;
      customFetch?: typeof fetch;
    },
  ) {
    this.modelId = modelId;
    this.provider = provider;
    this.baseURL = baseURL.replace(/\/$/, '');
    this.apiKey = options.apiKey;
    this.extraHeaders = options.headers ?? {};
    this.queryParams = options.queryParams;
    this.customFetch = options.customFetch ?? fetch;
  }

  private buildUrl(urlPath: string): string {
    let url = `${this.baseURL}${urlPath}`;
    if (this.queryParams) {
      url += `?${new URLSearchParams(this.queryParams).toString()}`;
    }
    return url;
  }

  private buildHeaders(extra?: Record<string, string | undefined>): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...this.extraHeaders,
    };
    if (this.apiKey) headers['Authorization'] = `Bearer ${this.apiKey}`;
    if (extra) {
      for (const [k, v] of Object.entries(extra)) {
        if (v !== undefined) headers[k] = v;
      }
    }
    return headers;
  }

  private buildRequestBody(options: LanguageModelV2CallOptions): Record<string, unknown> {
    const body: Record<string, unknown> = {
      model: this.modelId,
      messages: convertMessages(options.prompt),
      stream: false,
    };

    if (options.maxOutputTokens !== undefined) body.max_tokens = options.maxOutputTokens;
    if (options.temperature !== undefined) body.temperature = options.temperature;
    if (options.topP !== undefined) body.top_p = options.topP;
    if (options.frequencyPenalty !== undefined) body.frequency_penalty = options.frequencyPenalty;
    if (options.presencePenalty !== undefined) body.presence_penalty = options.presencePenalty;
    if (options.stopSequences !== undefined && options.stopSequences.length > 0) body.stop = options.stopSequences;
    if (options.seed !== undefined) body.seed = options.seed;

    const tools = convertTools(options.tools);
    if (tools) body.tools = tools;

    const toolChoice = convertToolChoice(options.toolChoice);
    if (toolChoice) body.tool_choice = toolChoice;

    if (options.responseFormat) {
      if (options.responseFormat.type === 'json') {
        body.response_format = { type: 'json_object' };
      }
    }

    return body;
  }

  private async doPost(options: LanguageModelV2CallOptions): Promise<{
    json: any;
    responseHeaders: Headers;
    bodyString: string;
  }> {
    const url = this.buildUrl('/chat/completions');
    const headers = this.buildHeaders(options.headers);
    const body = this.buildRequestBody(options);
    const bodyString = JSON.stringify(body);

    const response = await this.customFetch(url, {
      method: 'POST',
      headers,
      body: bodyString,
      signal: options.abortSignal,
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(`API request failed with status ${response.status}: ${text}`);
    }

    const json = await response.json();
    return { json, responseHeaders: response.headers, bodyString };
  }

  async doGenerate(options: LanguageModelV2CallOptions) {
    const { json, responseHeaders, bodyString } = await this.doPost(options);
    const choice = json.choices?.[0];

    return {
      content: parseResponseContent(choice ?? {}),
      finishReason: mapFinishReason(choice?.finish_reason),
      usage: parseUsage(json.usage),
      request: { body: bodyString },
      response: {
        id: json.id,
        modelId: json.model,
        timestamp: json.created ? new Date(json.created * 1000) : undefined,
        headers: Object.fromEntries(responseHeaders.entries()) as Record<string, string>,
        body: json,
      },
      warnings: [] as LanguageModelV2CallWarning[],
    };
  }

  async doStream(options: LanguageModelV2CallOptions) {
    const { json, responseHeaders, bodyString } = await this.doPost(options);
    const choice = json.choices?.[0];
    const content = parseResponseContent(choice ?? {});
    const finishReason = mapFinishReason(choice?.finish_reason);
    const usage = parseUsage(json.usage);

    const stream = new ReadableStream<LanguageModelV2StreamPart>({
      start(controller) {
        controller.enqueue({
          type: 'stream-start',
          warnings: [] as LanguageModelV2CallWarning[],
        });

        controller.enqueue({
          type: 'response-metadata',
          id: json.id,
          modelId: json.model,
          timestamp: json.created ? new Date(json.created * 1000) : undefined,
        });

        for (const block of content) {
          if (block.type === 'text') {
            const id = 'txt-0';
            controller.enqueue({ type: 'text-start', id });
            controller.enqueue({ type: 'text-delta', id, delta: block.text });
            controller.enqueue({ type: 'text-end', id });
          } else if (block.type === 'tool-call') {
            const id = block.toolCallId;
            controller.enqueue({
              type: 'tool-input-start',
              id,
              toolName: block.toolName,
            });
            controller.enqueue({
              type: 'tool-input-delta',
              id,
              delta: block.input,
            });
            controller.enqueue({
              type: 'tool-input-end',
              id,
            });
            controller.enqueue({
              type: 'tool-call',
              toolCallId: block.toolCallId,
              toolName: block.toolName,
              input: block.input,
            });
          }
        }

        controller.enqueue({
          type: 'finish',
          usage,
          finishReason,
        });

        controller.close();
      },
    });

    return {
      stream,
      request: { body: bodyString },
      response: {
        headers: Object.fromEntries(responseHeaders.entries()) as Record<string, string>,
      },
    };
  }
}

// ---------------------------------------------------------------------------
// Provider factory
// ---------------------------------------------------------------------------

export function createOpenAICompatible(options: MTLSProviderSettings) {
  let customFetch: typeof fetch | undefined;

  if ((options.clientCert && options.clientKey) || options.proxy || (options.removeKeys && options.removeKeys.length > 0)) {
    customFetch = createCustomFetch({
      clientCert: options.clientCert,
      clientKey: options.clientKey,
      caCert: options.caCert,
      proxy: options.proxy,
      removeKeys: options.removeKeys,
    });
  }

  const createModel = (modelId: string): LanguageModelV2 => {
    return new NoStreamChatLanguageModel(modelId, options.name, options.baseURL, {
      apiKey: options.apiKey,
      headers: options.headers,
      queryParams: options.queryParams,
      customFetch,
    });
  };

  const provider = (modelId: string) => createModel(modelId);
  provider.languageModel = createModel;
  provider.chatModel = createModel;

  return provider;
}

export default { createOpenAICompatible };
