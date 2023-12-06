sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://packages.opentofu.org/opentofu/tofu/gpgkey | sudo gpg --no-tty --batch --dearmor -o /etc/apt/keyrings/opentofu.gpg
sudo chmod a+r /etc/apt/keyrings/opentofu.gpg