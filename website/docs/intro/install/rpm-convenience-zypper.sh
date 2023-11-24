curl -s https://packagecloud.io/install/repositories/opentofu/tofu/script.rpm.sh?any=true -o /tmp/tofu-repository-setup.sh
# Inspect the downloaded script at /tmp/tofu-repository-setup.sh before running
sudo bash /tmp/tofu-repository-setup.sh
rm /tmp/tofu-repository-setup.sh

sudo zypper install -y tofu