"""OpenTofu middleware server implementation."""

import logging
from dataclasses import asdict
from typing import Any, Callable, Dict, List, Optional, TypeVar, Union

from .types import (
    Capability,
    HookResult,
    InitializeParams,
    InitializeResult,
    OnPlanCompletedParams,
    PostApplyParams,
    PostPlanParams,
    PostRefreshParams,
    PreApplyParams,
    PrePlanParams,
    PreRefreshParams,
)

# Type variable for handler functions
T = TypeVar("T")
Handler = Callable[[T], HookResult]
InitHandler = Callable[[InitializeParams], InitializeResult]


class MiddlewareServer:
    """Server for handling OpenTofu middleware requests."""

    def __init__(
        self,
        name: str,
        version: str = "1.0.0",
        logger: Optional[logging.Logger] = None,
    ) -> None:
        """Initialize a new middleware server.

        Args:
            name: The name of this middleware
            version: The version of this middleware
            logger: Optional logger instance
        """
        self.name = name
        self.version = version
        self.logger = logger or logging.getLogger(__name__)
        self.capabilities: List[Capability] = []

        # Handler storage
        self._initialize_handler: Optional[InitHandler] = None
        self._pre_plan_handler: Optional[Handler[PrePlanParams]] = None
        self._post_plan_handler: Optional[Handler[PostPlanParams]] = None
        self._pre_apply_handler: Optional[Handler[PreApplyParams]] = None
        self._post_apply_handler: Optional[Handler[PostApplyParams]] = None
        self._pre_refresh_handler: Optional[Handler[PreRefreshParams]] = None
        self._post_refresh_handler: Optional[Handler[PostRefreshParams]] = None
        self._on_plan_completed_handler: Optional[Handler[OnPlanCompletedParams]] = None

    def on_initialize(self, handler: InitHandler) -> "MiddlewareServer":
        """Register initialize handler."""
        self._initialize_handler = handler
        return self

    def pre_plan(self, handler: Handler[PrePlanParams]) -> "MiddlewareServer":
        """Register pre-plan handler."""
        self._pre_plan_handler = handler
        self.capabilities.append("pre-plan")
        return self

    def post_plan(self, handler: Handler[PostPlanParams]) -> "MiddlewareServer":
        """Register post-plan handler."""
        self._post_plan_handler = handler
        self.capabilities.append("post-plan")
        return self

    def pre_apply(self, handler: Handler[PreApplyParams]) -> "MiddlewareServer":
        """Register pre-apply handler."""
        self._pre_apply_handler = handler
        self.capabilities.append("pre-apply")
        return self

    def post_apply(self, handler: Handler[PostApplyParams]) -> "MiddlewareServer":
        """Register post-apply handler."""
        self._post_apply_handler = handler
        self.capabilities.append("post-apply")
        return self

    def pre_refresh(self, handler: Handler[PreRefreshParams]) -> "MiddlewareServer":
        """Register pre-refresh handler."""
        self._pre_refresh_handler = handler
        self.capabilities.append("pre-refresh")
        return self

    def post_refresh(self, handler: Handler[PostRefreshParams]) -> "MiddlewareServer":
        """Register post-refresh handler."""
        self._post_refresh_handler = handler
        self.capabilities.append("post-refresh")
        return self

    def on_plan_completed(
        self, handler: Handler[OnPlanCompletedParams]
    ) -> "MiddlewareServer":
        """Register on-plan-completed handler."""
        self._on_plan_completed_handler = handler
        self.capabilities.append("on-plan-completed")
        return self

    # Internal handler methods called by transport
    def _handle_initialize(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle initialize request."""
        init_params = InitializeParams(**params)
        
        if self._initialize_handler:
            result = self._initialize_handler(init_params)
        else:
            # Default handler
            result = InitializeResult(capabilities=self.capabilities)
        
        return asdict(result)

    def _handle_pre_plan(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle pre-plan request."""
        if not self._pre_plan_handler:
            return asdict(HookResult(status="pass"))
        
        pre_plan_params = PrePlanParams(**params)
        result = self._pre_plan_handler(pre_plan_params)
        return asdict(result)

    def _handle_post_plan(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle post-plan request."""
        if not self._post_plan_handler:
            return asdict(HookResult(status="pass"))
        
        post_plan_params = PostPlanParams(**params)
        result = self._post_plan_handler(post_plan_params)
        return asdict(result)

    def _handle_pre_apply(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle pre-apply request."""
        if not self._pre_apply_handler:
            return asdict(HookResult(status="pass"))
        
        pre_apply_params = PreApplyParams(**params)
        result = self._pre_apply_handler(pre_apply_params)
        return asdict(result)

    def _handle_post_apply(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle post-apply request."""
        if not self._post_apply_handler:
            return asdict(HookResult(status="pass"))
        
        post_apply_params = PostApplyParams(**params)
        result = self._post_apply_handler(post_apply_params)
        return asdict(result)

    def _handle_pre_refresh(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle pre-refresh request."""
        if not self._pre_refresh_handler:
            return asdict(HookResult(status="pass"))
        
        pre_refresh_params = PreRefreshParams(**params)
        result = self._pre_refresh_handler(pre_refresh_params)
        return asdict(result)

    def _handle_post_refresh(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle post-refresh request."""
        if not self._post_refresh_handler:
            return asdict(HookResult(status="pass"))
        
        post_refresh_params = PostRefreshParams(**params)
        result = self._post_refresh_handler(post_refresh_params)
        return asdict(result)

    def _handle_on_plan_completed(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle on-plan-completed request."""
        if not self._on_plan_completed_handler:
            return asdict(HookResult(status="pass"))
        
        on_plan_completed_params = OnPlanCompletedParams(**params)
        result = self._on_plan_completed_handler(on_plan_completed_params)
        return asdict(result)