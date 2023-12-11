#!/bin/bash

set -e

./install.sh $@

tofu --version
