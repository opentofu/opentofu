import type { PreviousMiddlewareMetadata } from "../types";

export function getMiddlewareData(
  previousMetadata: PreviousMiddlewareMetadata,
  middlewareName: string,
): any | undefined {
  return previousMetadata[middlewareName];
}

export function getChainedMiddleware(previousMetadata: PreviousMiddlewareMetadata): string[] {
  return Object.keys(previousMetadata);
}

export function hasMiddleware(
  previousMetadata: PreviousMiddlewareMetadata,
  middlewareName: string,
): boolean {
  return middlewareName in previousMetadata;
}

export function getChainPosition(previousMetadata: PreviousMiddlewareMetadata): number {
  return Object.keys(previousMetadata).length + 1;
}
