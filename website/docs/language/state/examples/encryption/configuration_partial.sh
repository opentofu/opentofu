TF_ENCRYPTION=$(cat <<EOF
key_provider "pbkdf2" "main" {
  passphrase = "correct-horse-battery-staple"
}
EOF
)
