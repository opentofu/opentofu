import type {
  HookResult,
  InitializeParams,
  InitializeResult,
  MiddlewareCapability,
  PostApplyParams,
  PostPlanParams,
  PostRefreshParams,
  PreApplyParams,
  PrePlanParams,
  PreRefreshParams,
} from "./types";

type HookHandler<T, R = HookResult> = (params: T) => Promise<R> | R;

export interface MiddlewareServerOptions {
  name: string;
  version?: string;
}

type PingHandler = () => Promise<{ message: string }> | { message: string };
type ShutdownHandler = () => Promise<void> | void;

// Type mapping for handlers
type HandlerMap = {
  initialize: HookHandler<InitializeParams, InitializeResult>;
  "pre-plan": HookHandler<PrePlanParams>;
  "post-plan": HookHandler<PostPlanParams>;
  "pre-apply": HookHandler<PreApplyParams>;
  "post-apply": HookHandler<PostApplyParams>;
  "pre-refresh": HookHandler<PreRefreshParams>;
  "post-refresh": HookHandler<PostRefreshParams>;
  ping: PingHandler;
  shutdown: ShutdownHandler;
};

type HandlerKey = keyof HandlerMap;

class TypedHandlerMap {
  private map = new Map<string, any>();

  set<K extends HandlerKey>(key: K, handler: HandlerMap[K]): void {
    this.map.set(key, handler);
  }

  get<K extends HandlerKey>(key: K): HandlerMap[K] | undefined {
    return this.map.get(key) as HandlerMap[K] | undefined;
  }

  has(key: string): boolean {
    return this.map.has(key);
  }
}

/**
 * Middleware server that defines handlers for OpenTofu hooks
 * Transport-agnostic - can be used with stdio, HTTP, etc.
 */
export class MiddlewareServer {
  private name: string;
  private version: string;
  private capabilities: Set<MiddlewareCapability> = new Set();
  private handlers = new TypedHandlerMap();

  constructor(options: MiddlewareServerOptions) {
    this.name = options.name;
    this.version = options.version || "1.0.0";
  }

  /**
   * Get the name of this middleware
   */
  getName(): string {
    return this.name;
  }

  /**
   * Get the version of this middleware
   */
  getVersion(): string {
    return this.version;
  }

  /**
   * Get the capabilities of this middleware
   */
  getCapabilities(): MiddlewareCapability[] {
    return Array.from(this.capabilities);
  }

  /**
   * Register a pre-plan hook handler
   */
  prePlan(handler: HookHandler<PrePlanParams>): this {
    this.capabilities.add("pre-plan");
    this.handlers.set("pre-plan", handler);
    return this;
  }

  /**
   * Register a post-plan hook handler
   */
  postPlan(handler: HookHandler<PostPlanParams>): this {
    this.capabilities.add("post-plan");
    this.handlers.set("post-plan", handler);
    return this;
  }

  /**
   * Register a pre-apply hook handler
   */
  preApply(handler: HookHandler<PreApplyParams>): this {
    this.capabilities.add("pre-apply");
    this.handlers.set("pre-apply", handler);
    return this;
  }

  /**
   * Register a post-apply hook handler
   */
  postApply(handler: HookHandler<PostApplyParams>): this {
    this.capabilities.add("post-apply");
    this.handlers.set("post-apply", handler);
    return this;
  }

  /**
   * Register a pre-refresh hook handler
   */
  preRefresh(handler: HookHandler<PreRefreshParams>): this {
    this.capabilities.add("pre-refresh");
    this.handlers.set("pre-refresh", handler);
    return this;
  }

  /**
   * Register a post-refresh hook handler
   */
  postRefresh(handler: HookHandler<PostRefreshParams>): this {
    this.capabilities.add("post-refresh");
    this.handlers.set("post-refresh", handler);
    return this;
  }

  /**
   * Register a custom initialize handler
   */
  onInitialize(handler: HookHandler<InitializeParams, InitializeResult>): this {
    this.handlers.set("initialize", handler);
    return this;
  }

  /**
   * Register a custom ping handler
   */
  onPing(handler: PingHandler): this {
    this.handlers.set("ping", handler);
    return this;
  }

  /**
   * Register a custom shutdown handler
   */
  onShutdown(handler: ShutdownHandler): this {
    this.handlers.set("shutdown", handler);
    return this;
  }

  /**
   * Handle a method call
   * Used by transports to process requests
   */
  async handleMethod(
    method: MiddlewareCapability | "initialize" | "ping" | "shutdown",
    params: any,
  ): Promise<any> {
    switch (method) {
      case "initialize": {
        const initHandler = this.handlers.get("initialize");
        if (initHandler) {
          return await initHandler(params);
        }
        // Default initialize response
        return {
          capabilities: this.getCapabilities(),
        };
      }

      case "pre-plan":
      case "post-plan":
      case "pre-apply":
      case "post-apply":
      case "pre-refresh":
      case "post-refresh": {
        const hookHandler = this.handlers.get(method as MiddlewareCapability);
        if (hookHandler) {
          return await hookHandler(params);
        }
        // Default to pass if no handler registered
        return { status: "pass" } as HookResult;
      }

      case "ping": {
        const pingHandler = this.handlers.get("ping");
        if (pingHandler) {
          return await pingHandler();
        }
        return { message: "pong" };
      }

      case "shutdown": {
        const shutdownHandler = this.handlers.get("shutdown");
        if (shutdownHandler) {
          await shutdownHandler();
        }
        return "ok";
      }

      default:
        throw new Error(`Method not found: ${method}`);
    }
  }

  /**
   * Check if this server handles a specific method
   */
  hasMethod(method: string): boolean {
    const builtInMethods = ["initialize", "ping", "shutdown"];
    return builtInMethods.includes(method) || this.handlers.has(method);
  }
}
