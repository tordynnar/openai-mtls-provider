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

/**
 * Creates a custom fetch function with mTLS support
 * Uses Bun's native TLS options for client certificate authentication
 */
function createMTLSFetch(options: {
  clientCert: string;
  clientKey: string;
  caCert: string;
}): typeof fetch {
  const certPath = resolvePath(options.clientCert);
  const keyPath = resolvePath(options.clientKey);
  const caPath = resolvePath(options.caCert);

  // Read certificates
  let cert: Buffer;
  let key: Buffer;
  let ca: Buffer;

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

  // Create custom fetch function that uses Bun's TLS options
  const mtlsFetch = async (
    input: string | URL | Request,
    init?: RequestInit
  ): Promise<Response> => {
    return fetch(input, {
      ...init,
      // @ts-ignore - Bun supports tls option for mTLS
      tls: {
        cert: cert.toString(),
        key: key.toString(),
        ca: ca.toString(),
        rejectUnauthorized: true,
      },
    });
  };

  return mtlsFetch;
}

/**
 * Creates an OpenAI-compatible provider with optional mTLS authentication.
 *
 * This function matches the signature expected by OpenCode's provider system.
 * When clientCert, clientKey, and caCert are provided, the provider will use
 * mTLS for all API requests.
 */
export function createOpenAICompatible(options: MTLSProviderSettings) {
  // Build the fetch function if mTLS certificates are provided
  let customFetch: typeof fetch | undefined;

  if (options.clientCert && options.clientKey && options.caCert) {
    customFetch = createMTLSFetch({
      clientCert: options.clientCert,
      clientKey: options.clientKey,
      caCert: options.caCert,
    });
  }

  // Create the base provider with optional mTLS fetch
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
