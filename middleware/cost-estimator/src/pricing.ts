interface ResourcePricing {
  monthlyBasePrice?: number;
  pricePerUnit?: number;
  unit?: string;
}

interface CloudPricing {
  [provider: string]: {
    [resourceType: string]: ResourcePricing;
  };
}

// Simplified pricing data - in a real implementation, this would come from
// cloud provider APIs or a pricing database
export const CLOUD_PRICING: CloudPricing = {
  aws: {
    aws_instance: {
      monthlyBasePrice: 50, // t3.medium as default
      pricePerUnit: 0.0416, // per hour
      unit: "hour",
    },
    aws_s3_bucket: {
      pricePerUnit: 0.023, // per GB per month
      unit: "GB",
    },
    aws_db_instance: {
      monthlyBasePrice: 100, // db.t3.medium
      pricePerUnit: 0.138,
      unit: "hour",
    },
    aws_lambda_function: {
      pricePerUnit: 0.00001667, // per GB-second
      unit: "GB-second",
    },
    aws_dynamodb_table: {
      pricePerUnit: 0.25, // per million read/write units
      unit: "million-requests",
    },
    aws_ebs_volume: {
      pricePerUnit: 0.10, // per GB per month for gp3
      unit: "GB",
    },
  },
  google: {
    google_compute_instance: {
      monthlyBasePrice: 45, // n1-standard-1
      pricePerUnit: 0.0475,
      unit: "hour",
    },
    google_storage_bucket: {
      pricePerUnit: 0.020, // per GB per month
      unit: "GB",
    },
    google_sql_database_instance: {
      monthlyBasePrice: 90,
      pricePerUnit: 0.125,
      unit: "hour",
    },
  },
  azurerm: {
    azurerm_virtual_machine: {
      monthlyBasePrice: 55, // Standard_B2s
      pricePerUnit: 0.0416,
      unit: "hour",
    },
    azurerm_storage_account: {
      pricePerUnit: 0.021, // per GB per month
      unit: "GB",
    },
    azurerm_sql_database: {
      monthlyBasePrice: 95,
      pricePerUnit: 0.13,
      unit: "hour",
    },
  },
};

export function getProviderFromResourceType(resourceType: string): string | null {
  // Extract provider from resource type (e.g., "aws_instance" -> "aws")
  const parts = resourceType.split("_");
  if (parts.length >= 2) {
    const provider = parts[0];
    if (provider === "google" || provider === "azurerm" || provider === "aws") {
      return provider;
    }
  }
  return null;
}

export function getResourcePricing(
  provider: string,
  resourceType: string,
): ResourcePricing | null {
  return CLOUD_PRICING[provider]?.[resourceType] || null;
}