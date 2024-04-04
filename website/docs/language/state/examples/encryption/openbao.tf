terraform {
  encryption {
    key_provider "openbao" "my_bao" {
      
      # Required. Name of the transit encryption key to use to encrypt/decrypt the datakey.
      key_name = "test-key"
      
      # Optional. Authorization Token to use when accessing OpenBao API.
      # Could be set via BAO_TOKEN environment variable.
      token = "s.Fg8wA4nDrP08TirpjEXkrTmt"

      # Optional. OpenBao server address to access the API.
      # Could be set via BAO_ADDR environment variable.
      address = "http://127.0.0.1:8200"

      # Optional. Path at whick Transit Secret Engine enabled in OpenBao.
      transit_engine_path = "/my-org/transit"

      # Optional. Number of bytes to generate as a key.
      key_length = 16
    }
  }
}
