import type { MiddlewareServer } from "./server";

/**
 * Base transport interface
 */
export interface Transport {
  /**
   * Connect the transport to a middleware server and start processing
   */
  connect(server: MiddlewareServer): Promise<void>;

  /**
   * Close the transport
   */
  close(): Promise<void>;
}
