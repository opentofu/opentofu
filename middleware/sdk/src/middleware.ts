import * as readline from "node:readline";
import {
  type HookResult,
  JsonRpcErrorCodes,
  type JsonRpcRequest,
  type JsonRpcResponse,
  type MethodParams,
  type MethodResult,
  type MiddlewareCapability,
} from "./types";

export interface MiddlewareHandlers {
  initialize?: (
    params: MethodParams["initialize"],
  ) => Promise<MethodResult["initialize"]> | MethodResult["initialize"];
  "pre-plan"?: (params: MethodParams["pre-plan"]) => Promise<HookResult> | HookResult;
  "post-plan"?: (params: MethodParams["post-plan"]) => Promise<HookResult> | HookResult;
  "pre-apply"?: (params: MethodParams["pre-apply"]) => Promise<HookResult> | HookResult;
  "post-apply"?: (params: MethodParams["post-apply"]) => Promise<HookResult> | HookResult;
  "pre-refresh"?: (params: MethodParams["pre-refresh"]) => Promise<HookResult> | HookResult;
  "post-refresh"?: (params: MethodParams["post-refresh"]) => Promise<HookResult> | HookResult;
  "on-plan-completed"?: (params: MethodParams["on-plan-completed"]) => Promise<HookResult> | HookResult;
  ping?: () => Promise<{ message: string }> | { message: string };
  shutdown?: () => Promise<void> | void;
}

export interface MiddlewareOptions {
  name: string;
  capabilities: MiddlewareCapability[];
  handlers: MiddlewareHandlers;
  logger?: (message: string) => void;
}

export class Middleware {
  private rl: readline.Interface;
  private options: MiddlewareOptions;

  constructor(options: MiddlewareOptions) {
    this.options = options;
    this.rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
      terminal: false,
    });
  }

  private log(message: string): void {
    if (this.options.logger) {
      this.options.logger(message);
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

    try {
      switch (method) {
        case "initialize":
          if (!this.options.handlers.initialize) {
            this.sendResponse(id, { capabilities: this.options.capabilities });
          } else {
            const result = await this.options.handlers.initialize(
              params as MethodParams["initialize"],
            );
            this.sendResponse(id, result);
          }
          break;

        case "pre-plan":
        case "post-plan":
        case "pre-apply":
        case "post-apply":
        case "pre-refresh":
        case "post-refresh":
        case "on-plan-completed": {
          const handler = this.options.handlers[method];
          if (!handler) {
            // If no handler provided, return pass by default
            this.sendResponse(id, { status: "pass" } as HookResult);
          } else {
            const result = await handler(params as any);
            this.sendResponse(id, result);
          }
          break;
        }

        case "ping": {
          if (!this.options.handlers.ping) {
            this.sendResponse(id, { message: "pong" });
          } else {
            const result = await this.options.handlers.ping();
            this.sendResponse(id, result);
          }
          break;
        }

        case "shutdown": {
          if (this.options.handlers.shutdown) {
            await this.options.handlers.shutdown();
          }
          this.sendResponse(id, "ok");
          // Give time for response to be sent
          setTimeout(() => process.exit(0), 100);
          break;
        }

        default: {
          this.sendError(id, JsonRpcErrorCodes.METHOD_NOT_FOUND, `Method not found: ${method}`);
          break;
        }
      }
    } catch (error) {
      this.log(`Error handling ${method}: ${error}`);
      this.sendError(
        id,
        JsonRpcErrorCodes.INTERNAL_ERROR,
        "Internal error",
        error instanceof Error ? error.message : String(error),
      );
    }
  }

  public start(): void {
    this.log(`${this.options.name} middleware started`);

    this.rl.on("line", async (line) => {
      try {
        const request = JSON.parse(line) as JsonRpcRequest;
        await this.handleRequest(request);
      } catch (error) {
        this.log(`Failed to parse request: ${error}`);
      }
    });
  }
}
