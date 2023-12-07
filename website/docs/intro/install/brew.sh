#!/bin/bash

set -ex

apt-get update
apt-get install -y curl git build-essential gcc procps curl file

/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

(echo; echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"') >> /root/.bashrc
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"

bash -x brew-install.sh

tofu --version
