terraform {
  encryption {
    key_provider "openbao" "my_bao" {
      
      # Required. Name of the transit encryption key
      # to use to encrypt/decrypt the data key.
      key_name = "test-key"
      
      # Optional. Authorization Token to use when accessing OpenBao API.
      # You can also set this in the BAO_TOKEN environment variable.
      token = "s.Fg8wA4nDrP08TirpjEXkrTmt"

      # Optional. OpenBao server address to access the API on.
      # You can also set this using the BAO_ADDR environment variable.
      address = "http://127.0.0.1:8200"

      # Optional. You can customize this if you mounted the
      # transit engine on a different path. Default: /transit
      transit_engine_path = "/my-org/transit"

      # Optional. Number of bytes to generate as a key. Default: 32
      key_length = 16
    }
  }
}
