"""Type definitions for OpenTofu middleware protocol."""

from dataclasses import dataclass, field
from typing import Any, Dict, List, Literal, Optional, Union

# Hook capability types
Capability = Literal[
    "pre-plan",
    "post-plan",
    "pre-apply",
    "post-apply",
    "pre-refresh",
    "post-refresh",
    "on-plan-completed",
]

# Resource modes
ResourceMode = Literal["managed", "data"]

# Hook status types
Status = Literal["pass", "fail"]


@dataclass
class InitializeParams:
    """Parameters for the initialize method."""

    version: str
    name: str


@dataclass
class InitializeResult:
    """Result of the initialize method."""

    capabilities: List[Capability]


@dataclass
class HookResult:
    """Result returned by all hook methods."""

    status: Status
    message: Optional[str] = None
    metadata: Optional[Dict[str, Any]] = None
    modified_config: Optional[Dict[str, Any]] = None  # Only for pre-plan


@dataclass
class PrePlanParams:
    """Parameters for pre-plan hook."""

    provider: str
    resource_type: str
    resource_name: str
    resource_mode: ResourceMode
    config: Optional[Dict[str, Any]] = None
    current_state: Optional[Dict[str, Any]] = None
    previous_middleware_metadata: Optional[Dict[str, Any]] = None


@dataclass
class PostPlanParams:
    """Parameters for post-plan hook."""

    provider: str
    resource_type: str
    resource_name: str
    resource_mode: ResourceMode
    planned_action: str
    current_state: Optional[Dict[str, Any]] = None
    planned_state: Optional[Dict[str, Any]] = None
    config: Optional[Dict[str, Any]] = None
    previous_middleware_metadata: Optional[Dict[str, Any]] = None


@dataclass
class PreApplyParams:
    """Parameters for pre-apply hook."""

    provider: str
    resource_type: str
    resource_name: str
    resource_mode: ResourceMode
    planned_action: str
    current_state: Optional[Dict[str, Any]] = None
    planned_state: Optional[Dict[str, Any]] = None
    config: Optional[Dict[str, Any]] = None
    previous_middleware_metadata: Optional[Dict[str, Any]] = None


@dataclass
class PostApplyParams:
    """Parameters for post-apply hook."""

    provider: str
    resource_type: str
    resource_name: str
    resource_mode: ResourceMode
    applied_action: str
    before: Optional[Dict[str, Any]] = None
    after: Optional[Dict[str, Any]] = None
    config: Optional[Dict[str, Any]] = None
    failed: bool = False
    previous_middleware_metadata: Optional[Dict[str, Any]] = None


@dataclass
class PreRefreshParams:
    """Parameters for pre-refresh hook."""

    provider: str
    resource_type: str
    resource_name: str
    resource_mode: ResourceMode
    current_state: Optional[Dict[str, Any]] = None
    previous_middleware_metadata: Optional[Dict[str, Any]] = None


@dataclass
class PostRefreshParams:
    """Parameters for post-refresh hook."""

    provider: str
    resource_type: str
    resource_name: str
    resource_mode: ResourceMode
    before: Optional[Dict[str, Any]] = None
    after: Optional[Dict[str, Any]] = None
    drift_detected: bool = False
    previous_middleware_metadata: Optional[Dict[str, Any]] = None


@dataclass
class OnPlanCompletedParams:
    """Parameters for on-plan-completed hook."""

    plan_json: Dict[str, Any]
    success: bool
    errors: Optional[List[str]] = None
    previous_middleware_metadata: Optional[Dict[str, Any]] = None