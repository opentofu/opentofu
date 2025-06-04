import * as readline from "node:readline";
import type { MiddlewareServer } from "./server";
import type { Transport } from "./transport";
import {
  JsonRpcErrorCodes,
  type JsonRpcRequest,
  type JsonRpcResponse,
  type MiddlewareCapability,
} from "./types";

export interface StdioTransportOptions {
  logger?: (message: string) => void;
}

/**
 * Transport that communicates via stdin/stdout using JSON-RPC
 */
export class StdioTransport implements Transport {
  private rl?: readline.Interface;
  private server?: MiddlewareServer;
  private logger?: (message: string) => void;
  private shutdownTimeout = 100;

  constructor(options: StdioTransportOptions = {}) {
    this.logger = options.logger;
  }

  private log(message: string): void {
    if (this.logger) {
      this.logger(`[StdioTransport] ${message}`);
    }
  }

  private sendResponse<T>(id: number | string, result?: T, error?: any): void {
    const response: JsonRpcResponse<T> = {
      jsonrpc: "2.0",
      id,
    };

    if (error) {
      response.error = error;
    } else {
      response.result = result;
    }

    console.log(JSON.stringify(response));
  }

  private sendError(id: number | string, code: number, message: string, data?: any): void {
    this.sendResponse(id, undefined, { code, message, data });
  }

  private async handleRequest(request: JsonRpcRequest): Promise<void> {
    const { method, params, id } = request;

    if (!this.server) {
      this.sendError(id, JsonRpcErrorCodes.INTERNAL_ERROR, "Server not connected");
      return;
    }

    try {
      this.log(`Handling ${method} request`);
      const result = await this.server.handleMethod(method as MiddlewareCapability, params);

      // Special handling for shutdown
      if (method === "shutdown") {
        this.sendResponse(id, result);
        this.log("Shutting down after response");
        setTimeout(() => process.exit(0), this.shutdownTimeout);
        return;
      }

      this.sendResponse(id, result);
    } catch (error) {
      this.log(`Error handling ${method}: ${error}`);

      const errorMessage = error instanceof Error ? error.message : String(error);

      if (errorMessage.includes("Method not found")) {
        this.sendError(id, JsonRpcErrorCodes.METHOD_NOT_FOUND, errorMessage);
      } else {
        this.sendError(id, JsonRpcErrorCodes.INTERNAL_ERROR, "Internal error", errorMessage);
      }
    }
  }

  async connect(server: MiddlewareServer): Promise<void> {
    this.server = server;
    this.log(`Connected to server: ${server.getName()} v${server.getVersion()}`);

    this.rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
      terminal: false,
    });

    this.rl.on("line", async (line) => {
      try {
        const request = JSON.parse(line) as JsonRpcRequest;
        await this.handleRequest(request);
      } catch (error) {
        this.log(`Failed to parse request: ${error}`);
        // Can't send error response without request ID
      }
    });

    return new Promise(() => {
      // Do nothing, just keep the event loop running
    });
  }

  async close(): Promise<void> {
    if (this.rl) {
      this.rl.close();
    }
  }

  /**
   * Set the timeout before process exit after shutdown (milliseconds)
   */
  setShutdownTimeout(timeout: number): this {
    this.shutdownTimeout = timeout;
    return this;
  }
}
