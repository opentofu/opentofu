$Env:TF_ENCRYPTION = @"
key_provider "pbkdf2" "main" {
  passphrase = "correct-horse-battery-staple"
}
"@