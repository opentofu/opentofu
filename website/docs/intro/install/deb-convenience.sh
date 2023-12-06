curl --proto '=https' --tlsv1.2 -fsSL 'https://packages.opentofu.org/install/repositories/opentofu/tofu/script.deb.sh?any=true' -o /tmp/tofu-repository-setup.sh
# Inspect the downloaded script at /tmp/tofu-repository-setup.sh before running
sudo bash /tmp/tofu-repository-setup.sh
rm /tmp/tofu-repository-setup.sh

sudo apt-get install tofu