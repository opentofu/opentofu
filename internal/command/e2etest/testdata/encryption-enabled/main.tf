terraform {
        encryption {
                key_provider "pbkdf2" "basic" {
                        passphrase = "26281afb-83f1-47ec-9b2d-2aebf6417167"
                        key_length = 32
                        iterations = 200000
                        hash_function = "sha512"
                        salt_length = 12
                }
                method "aes_gcm" "example" {
                        keys = key_provider.pbkdf2.basic
                }
                backend {
                        method = method.aes_gcm.example
                }
        }
}

variable "iter" {
	type = string
}

resource "tfcoremock_simple_resource" "simple" {
        string = "helloworld ${var.iter}"
}
