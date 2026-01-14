import { createOpenAICompatible as createBaseProvider } from '@ai-sdk/openai-compatible';
import * as fs from 'fs';
import * as path from 'path';

export interface MTLSProviderSettings {
  /**
   * Base URL for the API calls.
   */
  baseURL: string;

  /**
   * Provider name.
   */
  name: string;

  /**
   * API key for authenticating requests.
   */
  apiKey?: string;

  /**
   * Optional custom headers to include in requests.
   */
  headers?: Record<string, string>;

  /**
   * Optional custom url query parameters to include in request urls.
   */
  queryParams?: Record<string, string>;

  /**
   * Path to the client certificate file (PEM format)
   */
  clientCert?: string;

  /**
   * Path to the client key file (PEM format)
   */
  clientKey?: string;

  /**
   * Path to the CA certificate file (PEM format)
   */
  caCert?: string;

  /**
   * HTTP proxy URL (e.g., "http://localhost:8080")
   */
  proxy?: string;
}

/**
 * Resolves a path relative to the current working directory
 */
function resolvePath(p: string): string {
  if (path.isAbsolute(p)) {
    return p;
  }
  return path.resolve(process.cwd(), p);
}

interface FetchOptions {
  clientCert?: string;
  clientKey?: string;
  caCert?: string;
  proxy?: string;
}

/**
 * Creates a custom fetch function with mTLS and/or proxy support
 * Uses Bun's native TLS and proxy options
 */
function createCustomFetch(options: FetchOptions): typeof fetch {
  let cert: Buffer | undefined;
  let key: Buffer | undefined;
  let ca: Buffer | undefined;

  // Load certificates if provided
  if (options.clientCert && options.clientKey && options.caCert) {
    const certPath = resolvePath(options.clientCert);
    const keyPath = resolvePath(options.clientKey);
    const caPath = resolvePath(options.caCert);

    try {
      cert = fs.readFileSync(certPath);
    } catch (e) {
      throw new Error(`Failed to read client certificate from ${certPath}: ${e}`);
    }

    try {
      key = fs.readFileSync(keyPath);
    } catch (e) {
      throw new Error(`Failed to read client key from ${keyPath}: ${e}`);
    }

    try {
      ca = fs.readFileSync(caPath);
    } catch (e) {
      throw new Error(`Failed to read CA certificate from ${caPath}: ${e}`);
    }
  }

  // Create custom fetch function
  const customFetch = async (
    input: string | URL | Request,
    init?: RequestInit
  ): Promise<Response> => {
    const fetchOptions: RequestInit & { tls?: object; proxy?: string } = {
      ...init,
    };

    // Add TLS options if certificates are provided
    if (cert && key && ca) {
      // @ts-ignore - Bun supports tls option for mTLS
      fetchOptions.tls = {
        cert: cert.toString(),
        key: key.toString(),
        ca: ca.toString(),
        rejectUnauthorized: true,
      };
    }

    // Add proxy if provided
    if (options.proxy) {
      // @ts-ignore - Bun supports proxy option
      fetchOptions.proxy = options.proxy;
    }

    return fetch(input, fetchOptions);
  };

  return customFetch;
}

/**
 * Creates an OpenAI-compatible provider with optional mTLS authentication and proxy support.
 *
 * This function matches the signature expected by OpenCode's provider system.
 * When clientCert, clientKey, and caCert are provided, the provider will use
 * mTLS for all API requests.
 * When proxy is provided, all requests will be routed through the HTTP proxy.
 */
export function createOpenAICompatible(options: MTLSProviderSettings) {
  // Build the fetch function if mTLS certificates or proxy are provided
  let customFetch: typeof fetch | undefined;

  const hasMTLS = options.clientCert && options.clientKey && options.caCert;
  const hasProxy = !!options.proxy;

  if (hasMTLS || hasProxy) {
    customFetch = createCustomFetch({
      clientCert: options.clientCert,
      clientKey: options.clientKey,
      caCert: options.caCert,
      proxy: options.proxy,
    });
  }

  // Create the base provider with optional custom fetch
  return createBaseProvider({
    baseURL: options.baseURL,
    name: options.name,
    apiKey: options.apiKey,
    headers: options.headers,
    queryParams: options.queryParams,
    fetch: customFetch,
  });
}

/**
 * Default export for CommonJS compatibility
 */
export default { createOpenAICompatible };
