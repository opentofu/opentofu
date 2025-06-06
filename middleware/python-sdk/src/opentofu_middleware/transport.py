"""Transport layer for OpenTofu middleware communication."""

import json
import sys
from abc import ABC, abstractmethod
from typing import TYPE_CHECKING, Any, Dict, Optional

if TYPE_CHECKING:
    from .server import MiddlewareServer


class Transport(ABC):
    """Abstract base class for middleware transports."""

    @abstractmethod
    def connect(self, server: "MiddlewareServer") -> None:
        """Connect the transport to a middleware server."""
        pass

    @abstractmethod
    def start(self) -> None:
        """Start listening for requests."""
        pass

    @abstractmethod
    def stop(self) -> None:
        """Stop listening for requests."""
        pass


class StdioTransport(Transport):
    """JSON-RPC transport over stdin/stdout."""

    def __init__(self) -> None:
        self.server: Optional["MiddlewareServer"] = None
        self.running = False

    def connect(self, server: "MiddlewareServer") -> None:
        """Connect the transport to a middleware server."""
        self.server = server

    def start(self) -> None:
        """Start listening for JSON-RPC requests on stdin."""
        if not self.server:
            raise RuntimeError("Transport not connected to a server")

        self.running = True
        while self.running:
            try:
                line = sys.stdin.readline()
                if not line:
                    break

                request = json.loads(line.strip())
                response = self._handle_request(request)
                
                # Send response
                sys.stdout.write(json.dumps(response) + "\n")
                sys.stdout.flush()

            except json.JSONDecodeError as e:
                # Send error response
                error_response = {
                    "jsonrpc": "2.0",
                    "error": {
                        "code": -32700,
                        "message": f"Parse error: {str(e)}",
                    },
                    "id": None,
                }
                sys.stdout.write(json.dumps(error_response) + "\n")
                sys.stdout.flush()
            except Exception as e:
                # Log error to stderr
                sys.stderr.write(f"Unexpected error: {str(e)}\n")
                sys.stderr.flush()

    def stop(self) -> None:
        """Stop listening for requests."""
        self.running = False

    def _handle_request(self, request: Dict[str, Any]) -> Dict[str, Any]:
        """Handle a JSON-RPC request and return a response."""
        if not self.server:
            return self._error_response(
                request.get("id"), -32603, "Server not initialized"
            )

        method = request.get("method")
        params = request.get("params", {})
        request_id = request.get("id")

        try:
            # Route to appropriate handler
            if method == "initialize":
                result = self.server._handle_initialize(params)
            elif method == "pre-plan":
                result = self.server._handle_pre_plan(params)
            elif method == "post-plan":
                result = self.server._handle_post_plan(params)
            elif method == "pre-apply":
                result = self.server._handle_pre_apply(params)
            elif method == "post-apply":
                result = self.server._handle_post_apply(params)
            elif method == "pre-refresh":
                result = self.server._handle_pre_refresh(params)
            elif method == "post-refresh":
                result = self.server._handle_post_refresh(params)
            elif method == "on-plan-completed":
                result = self.server._handle_on_plan_completed(params)
            elif method == "ping":
                result = {"message": "pong"}
            elif method == "shutdown":
                result = "ok"
                self.stop()
            else:
                return self._error_response(
                    request_id, -32601, f"Method not found: {method}"
                )

            return {
                "jsonrpc": "2.0",
                "result": result,
                "id": request_id,
            }

        except Exception as e:
            return self._error_response(
                request_id, -32603, f"Internal error: {str(e)}"
            )

    def _error_response(
        self, request_id: Optional[Any], code: int, message: str
    ) -> Dict[str, Any]:
        """Create a JSON-RPC error response."""
        return {
            "jsonrpc": "2.0",
            "error": {
                "code": code,
                "message": message,
            },
            "id": request_id,
        }