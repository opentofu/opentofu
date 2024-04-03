terraform {
  encryption {
    key_provider "openbao" "my_bao" {
      token = "s.Fg8wA4nDrP08TirpjEXkrTmt"
      address = "http://127.0.0.1:8200"
      key_name = "test-key"
      key_length = 16
    }
  }
}
