$Env:TF_ENCRYPTION = @"
key_provider "some_key_provider" "some_name" {
  # Key provider options here
}

method "some_method_type" "some_method_name" {
  # Method options here
  keys = key_provider.some_key_provider.some_name
}

state {
  # Encryption/decryption for state data
  method = method.some_method_type.some_method_name
}

plan {
  # Encryption/decryption for plan data
  method = method.some_method_type.some_method_name
}

remote_state_data_sources {
  # See below
}
"@