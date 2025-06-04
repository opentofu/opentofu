import { getProviderFromResourceType, getResourcePricing } from "./pricing";

export interface CostEstimate {
  monthly: number;
  currency: string;
  confidence: "high" | "medium" | "low";
  breakdown?: {
    [key: string]: number;
  };
}

/**
 * Estimate the cost of a resource based on its type and configuration
 */
export function estimateResourceCost(
  resourceType: string,
  config: any,
  tags?: Record<string, string>,
): CostEstimate | null {
  const provider = getProviderFromResourceType(resourceType);
  if (!provider) {
    return null;
  }

  const pricing = getResourcePricing(provider, resourceType);
  if (!pricing) {
    return null;
  }

  let monthlyCost = pricing.monthlyBasePrice || 0;
  const breakdown: Record<string, number> = {};

  // Resource-specific cost calculations
  switch (resourceType) {
    case "aws_s3_bucket": {
      const storageGB = extractStorageSize(config, tags);
      const storageCost = storageGB * (pricing.pricePerUnit || 0);
      monthlyCost += storageCost;
      breakdown.storage = storageCost;

      // Estimate request costs if provided in tags
      const monthlyRequests = extractMonthlyRequests(tags);
      const requestCost = (monthlyRequests / 1000) * 0.0004; // $0.0004 per 1000 requests
      monthlyCost += requestCost;
      if (requestCost > 0) {
        breakdown.requests = requestCost;
      }
      break;
    }

    case "aws_instance":
    case "google_compute_instance":
    case "azurerm_virtual_machine": {
      // Check instance type and adjust base price
      const instanceType = config.instance_type || config.machine_type || config.vm_size;
      if (instanceType) {
        monthlyCost = getInstancePrice(provider, instanceType);
      }

      // Add storage costs if applicable
      const storageGB = config.root_block_device?.volume_size || config.boot_disk?.size || 0;
      const storageCost = storageGB * 0.1; // $0.10 per GB
      monthlyCost += storageCost;
      if (storageCost > 0) {
        breakdown.storage = storageCost;
      }
      break;
    }

    case "aws_db_instance":
    case "google_sql_database_instance":
    case "azurerm_sql_database": {
      // Check for multi-AZ/HA
      if (config.multi_az || config.availability_type === "REGIONAL") {
        monthlyCost *= 2;
        breakdown.highAvailability = monthlyCost / 2;
      }

      // Add storage costs
      const storageGB = config.allocated_storage || config.disk_size || 20;
      const storageCost = storageGB * 0.115; // $0.115 per GB
      monthlyCost += storageCost;
      breakdown.storage = storageCost;
      break;
    }

    case "aws_ebs_volume": {
      const sizeGB = config.size || 8;
      const volumeType = config.type || "gp3";
      const pricePerGB = volumeType === "io2" ? 0.125 : 0.08;
      monthlyCost = sizeGB * pricePerGB;
      breakdown.storage = monthlyCost;
      break;
    }
  }

  // Determine confidence level
  let confidence: "high" | "medium" | "low" = "medium";
  if (monthlyCost === 0 || !pricing.pricePerUnit) {
    confidence = "low";
  } else if (breakdown && Object.keys(breakdown).length > 1) {
    confidence = "high";
  }

  return {
    monthly: Math.round(monthlyCost * 100) / 100, // Round to 2 decimal places
    currency: "USD",
    confidence,
    breakdown: Object.keys(breakdown).length > 0 ? breakdown : undefined,
  };
}

function extractStorageSize(config: any, tags?: Record<string, string>): number {
  // Check tags first for estimated storage
  if (tags?.EstimatedStorageGB) {
    return Number.parseInt(tags.EstimatedStorageGB, 10) || 0;
  }

  // Check common storage configuration fields
  return config.size || config.storage || config.disk_size || 100; // Default 100GB
}

function extractMonthlyRequests(tags?: Record<string, string>): number {
  if (!tags) return 0;

  const getRequests = Number.parseInt(tags.EstimatedMonthlyGETRequests || "0", 10);
  const putRequests = Number.parseInt(tags.EstimatedMonthlyPUTRequests || "0", 10);

  return getRequests + putRequests;
}

function getInstancePrice(provider: string, instanceType: string): number {
  // Simplified instance pricing - in reality, this would be a large lookup table
  const instancePrices: Record<string, Record<string, number>> = {
    aws: {
      "t3.micro": 7.5,
      "t3.small": 15,
      "t3.medium": 30,
      "t3.large": 60,
      "m5.large": 70,
      "m5.xlarge": 140,
      "c5.large": 85,
      "c5.xlarge": 170,
    },
    google: {
      "e2-micro": 6,
      "e2-small": 12,
      "e2-medium": 24,
      "n1-standard-1": 45,
      "n1-standard-2": 90,
      "n1-standard-4": 180,
    },
    azurerm: {
      Standard_B1s: 10,
      Standard_B2s: 40,
      Standard_D2s_v3: 70,
      Standard_D4s_v3: 140,
    },
  };

  return instancePrices[provider]?.[instanceType] || 50; // Default to $50/month
}
