"""Utility classes and functions for OpenTofu middleware."""

import json
import logging
import sys
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, Optional


class FileLogger:
    """Simple file logger for middleware debugging."""

    def __init__(self, log_file: Optional[str] = None) -> None:
        """Initialize file logger.

        Args:
            log_file: Path to log file. If None, logs to stderr.
        """
        self.log_file = Path(log_file) if log_file else None
        
        # Configure logging
        if self.log_file:
            logging.basicConfig(
                level=logging.DEBUG,
                format="%(asctime)s [%(levelname)s] %(message)s",
                handlers=[
                    logging.FileHandler(self.log_file),
                    logging.StreamHandler(sys.stderr),
                ],
            )
        else:
            logging.basicConfig(
                level=logging.DEBUG,
                format="%(asctime)s [%(levelname)s] %(message)s",
                handlers=[logging.StreamHandler(sys.stderr)],
            )
        
        self.logger = logging.getLogger("opentofu_middleware")

    def debug(self, message: str, **kwargs: Any) -> None:
        """Log debug message."""
        self.logger.debug(message, extra=kwargs)

    def info(self, message: str, **kwargs: Any) -> None:
        """Log info message."""
        self.logger.info(message, extra=kwargs)

    def warning(self, message: str, **kwargs: Any) -> None:
        """Log warning message."""
        self.logger.warning(message, extra=kwargs)

    def error(self, message: str, **kwargs: Any) -> None:
        """Log error message."""
        self.logger.error(message, extra=kwargs)


class ResourceUtils:
    """Utilities for working with OpenTofu resources."""

    @staticmethod
    def get_resource_address(
        resource_type: str, resource_name: str, module_address: Optional[str] = None
    ) -> str:
        """Get the full resource address.

        Args:
            resource_type: The resource type (e.g., "aws_instance")
            resource_name: The resource name (e.g., "web")
            module_address: Optional module address

        Returns:
            Full resource address
        """
        base_address = f"{resource_type}.{resource_name}"
        if module_address:
            return f"{module_address}.{base_address}"
        return base_address

    @staticmethod
    def parse_provider(provider: str) -> Dict[str, str]:
        """Parse provider string into components.

        Args:
            provider: Provider string (e.g., "registry.opentofu.org/hashicorp/aws")

        Returns:
            Dict with registry, namespace, and type
        """
        parts = provider.split("/")
        if len(parts) == 3:
            return {
                "registry": parts[0],
                "namespace": parts[1],
                "type": parts[2],
            }
        elif len(parts) == 2:
            return {
                "registry": "registry.opentofu.org",
                "namespace": parts[0],
                "type": parts[1],
            }
        else:
            return {
                "registry": "registry.opentofu.org",
                "namespace": "hashicorp",
                "type": provider,
            }

    @staticmethod
    def extract_tags(config: Optional[Dict[str, Any]]) -> Dict[str, str]:
        """Extract tags from resource configuration.

        Args:
            config: Resource configuration

        Returns:
            Dictionary of tags
        """
        if not config:
            return {}
        
        tags = config.get("tags", {})
        if isinstance(tags, dict):
            return tags
        return {}


class CostEstimator:
    """Base class for cost estimation."""

    # Simple cost database for common resources
    RESOURCE_COSTS = {
        "aws_instance": {
            "t2.micro": 0.0116,
            "t2.small": 0.023,
            "t2.medium": 0.0464,
            "t2.large": 0.0928,
            "t3.micro": 0.0104,
            "t3.small": 0.0208,
            "t3.medium": 0.0416,
            "t3.large": 0.0832,
            "m5.large": 0.096,
            "m5.xlarge": 0.192,
        },
        "aws_s3_bucket": {
            "STANDARD": 0.023,  # per GB
            "INTELLIGENT_TIERING": 0.0125,
            "GLACIER": 0.004,
        },
        "aws_rds_instance": {
            "db.t3.micro": 0.017,
            "db.t3.small": 0.034,
            "db.t3.medium": 0.068,
            "db.m5.large": 0.171,
            "db.m5.xlarge": 0.342,
        },
        "google_compute_instance": {
            "n1-standard-1": 0.0475,
            "n1-standard-2": 0.095,
            "n1-standard-4": 0.19,
            "e2-micro": 0.0084,
            "e2-small": 0.0168,
            "e2-medium": 0.0336,
        },
        "azurerm_virtual_machine": {
            "Standard_B1s": 0.0104,
            "Standard_B2s": 0.0416,
            "Standard_D2s_v3": 0.096,
            "Standard_D4s_v3": 0.192,
        },
    }

    @classmethod
    def estimate_resource_cost(
        cls,
        resource_type: str,
        config: Optional[Dict[str, Any]],
        tags: Optional[Dict[str, str]] = None,
    ) -> Dict[str, Any]:
        """Estimate cost for a resource.

        Args:
            resource_type: The resource type
            config: Resource configuration
            tags: Resource tags (may contain cost hints)

        Returns:
            Cost estimation dictionary
        """
        if not config:
            return {"hourly": 0, "monthly": 0, "confidence": "low"}

        hourly_cost = 0
        confidence = "medium"

        # Instance types
        if resource_type == "aws_instance":
            instance_type = config.get("instance_type", "t2.micro")
            hourly_cost = cls.RESOURCE_COSTS.get("aws_instance", {}).get(
                instance_type, 0.05
            )
        
        elif resource_type == "aws_s3_bucket":
            # Check tags for storage hints
            if tags:
                estimated_gb = float(tags.get("EstimatedStorageGB", "100"))
                storage_class = tags.get("StorageClass", "STANDARD")
                gb_cost = cls.RESOURCE_COSTS.get("aws_s3_bucket", {}).get(
                    storage_class, 0.023
                )
                hourly_cost = (gb_cost * estimated_gb) / 730  # Convert to hourly
                confidence = "high"  # User provided estimates
            else:
                hourly_cost = 0.023 * 100 / 730  # Assume 100GB
                confidence = "low"
        
        elif resource_type == "aws_rds_instance":
            instance_class = config.get("instance_class", "db.t3.micro")
            hourly_cost = cls.RESOURCE_COSTS.get("aws_rds_instance", {}).get(
                instance_class, 0.05
            )
        
        elif resource_type == "google_compute_instance":
            machine_type = config.get("machine_type", "e2-micro")
            hourly_cost = cls.RESOURCE_COSTS.get("google_compute_instance", {}).get(
                machine_type, 0.05
            )
        
        elif resource_type == "azurerm_virtual_machine":
            size = config.get("size", "Standard_B1s")
            hourly_cost = cls.RESOURCE_COSTS.get("azurerm_virtual_machine", {}).get(
                size, 0.05
            )

        monthly_cost = hourly_cost * 730

        return {
            "hourly": round(hourly_cost, 4),
            "monthly": round(monthly_cost, 2),
            "currency": "USD",
            "confidence": confidence,
            "timestamp": datetime.utcnow().isoformat() + "Z",
        }