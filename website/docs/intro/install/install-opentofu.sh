#!/bin/sh
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


# OpenTofu Installer
#
# This script installs OpenTofu via any of the supported methods.

export TOFU_INSTALL_EXIT_CODE_OK=0
export TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET=1
export TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED=2
export TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT=3

export TOFU_INSTALL_RETURN_CODE_COMMAND_NOT_FOUND=11
export TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED=13

bold=""
normal=""
red=""
green=""
yellow=""
blue=""
magenta=""
cyan=""
gray=""
if [ -t 1 ]; then
    if command -v "tput" >/dev/null 2>&1; then
      colors=$(tput colors)
    else
      colors=2
    fi

    if [ "${colors}" -ge 8 ]; then
        bold="$(tput bold)"
        normal="$(tput sgr0)"
        red="$(tput setaf 1)"
        green="$(tput setaf 2)"
        yellow="$(tput setaf 3)"
        blue="$(tput setaf 4)"
        magenta="$(tput setaf 5)"
        cyan="$(tput setaf 6)"
        gray="$(tput setaf 245)"
    fi
fi

ROOT_METHOD=auto
INSTALL_METHOD=""
DEFAULT_INSTALL_PATH=/opt/opentofu
INSTALL_PATH="${DEFAULT_INSTALL_PATH}"
DEFAULT_SYMLINK_PATH=/usr/local/bin
SYMLINK_PATH="${DEFAULT_SYMLINK_PATH}"
DEFAULT_OPENTOFU_VERSION=latest
OPENTOFU_VERSION="${DEFAULT_OPENTOFU_VERSION}"
DEFAULT_DEB_GPG_URL=https://get.opentofu.org/opentofu.gpg
DEB_GPG_URL="${DEFAULT_DEB_GPG_URL}"
DEFAULT_DEB_REPO_GPG_URL=https://packages.opentofu.org/opentofu/tofu/gpgkey
DEB_REPO_GPG_URL="${DEFAULT_DEB_REPO_GPG_URL}"
DEFAULT_DEB_REPO_URL=https://packages.opentofu.org/opentofu/tofu/any/
DEB_REPO_URL=${DEFAULT_DEB_REPO_URL}
DEFAULT_DEB_REPO_SUITE=any
DEB_REPO_SUITE="${DEFAULT_DEB_REPO_SUITE}"
DEFAULT_DEB_REPO_COMPONENTS=main
DEB_REPO_COMPONENTS="${DEFAULT_DEB_REPO_COMPONENTS}"
DEFAULT_RPM_REPO_URL=https://packages.opentofu.org/opentofu/tofu/rpm_any/rpm_any/

RPM_REPO_URL=${DEFAULT_RPM_REPO_URL}
DEFAULT_RPM_REPO_GPG_URL=https://packages.opentofu.org/opentofu/tofu/gpgkey
DEFAULT_RPM_GPG_URL=https://get.opentofu.org/opentofu.asc
RPM_GPG_URL="${DEFAULT_RPM_GPG_URL}"
RPM_REPO_GPG_URL="${DEFAULT_RPM_REPO_GPG_URL}"
#TODO once the package makes it into stable change this to "-"
DEFAULT_APK_REPO_URL="@testing https://dl-cdn.alpinelinux.org/alpine/edge/testing"
APK_REPO_URL=${DEFAULT_APK_REPO_URL}
DEFAULT_APK_PACKAGE="opentofu@testing"
APK_PACKAGE="${DEFAULT_APK_PACKAGE}"
DEFAULT_COSIGN_PATH=cosign
COSIGN_PATH=${DEFAULT_COSIGN_PATH}
DEFAULT_COSIGN_IDENTITY=autodetect
COSIGN_IDENTITY=${DEFAULT_COSIGN_IDENTITY}
DEFAULT_COSIGN_OIDC_ISSUER=https://token.actions.githubusercontent.com
COSIGN_OIDC_ISSUER=${DEFAULT_COSIGN_OIDC_ISSUER}
SKIP_VERIFY=0

# region ZSH
if [ -n "${ZSH_VERSION}" ]; then
  ## Enable POSIX-style word splitting:
  setopt SH_WORD_SPLIT >/dev/null 2>&1
fi
# endregion

log_success() {
  if [ -z "$1" ]; then
    return
  fi
  echo "${green}$1${normal}" 1>&2
}

log_warning() {
  if [ -z "$1" ]; then
    return
  fi
  echo "${yellow}$1${normal}" 1>&2
}

log_info() {
  if [ -z "$1" ]; then
    return
  fi
  echo "${cyan}$1${normal}" 1>&2
}

log_debug() {
  if [ -z "$1" ]; then
    return
  fi
  if [ -z "${LOG_DEBUG}" ]; then
    return
  fi
  echo "${gray}$1${normal}" 1>&2
}

log_error() {
  if [ -z "$1" ]; then
    return
  fi
  echo "${red}$1${normal}" 1>&2
}

# This function checks if the command specified in $1 exists.
command_exists() {
  log_debug "Determining if the ${1} command is available..."
  if [ -z "$1" ]; then
    log_error "Bug: no command supplied to command_exists()"
    return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
  fi
  if ! command -v "$1" >/dev/null 2>&1; then
    log_debug "The ${1} command is not available."
    return "${TOFU_INSTALL_RETURN_CODE_COMMAND_NOT_FOUND}"
  fi
  log_debug "The ${1} command is available."
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

is_root() {
  if [ "$(id -u || true)" -eq 0 ]; then
    return 0
  fi
  return 1
}

# This function runs the specified command as root.
as_root() {
  # shellcheck disable=SC2145
  log_debug "Running command as root: $*"
  case "${ROOT_METHOD}" in
    auto)
      log_debug "Automatically determining root method..."
      if is_root; then
        log_debug "We are already root, no user change needed."
        "$@"
      elif command_exists "sudo"; then
        log_debug "Running command using sudo."
        sudo "$@"
      elif command_exists "su"; then
        log_debug "Running command using su."
        su root "$@"
      else
        log_error "Neither su nor sudo is installed, cannot obtain root privileges."
        return "${TOFU_INSTALL_RETURN_CODE_COMMAND_NOT_FOUND}"
      fi
      return $?
      ;;
    none)
      log_debug "Using manual root method 'none'."
      "$@"
      return $?
      ;;
    sudo)
      log_debug "Using manual root method 'sudo'."
      sudo "$@"
      return $?
      ;;
    su)
      log_debug "Using manual root method 'su'."
      su root "$@"
      return $?
      ;;
    *)
      log_error "Bug: invalid root method value: $1"
      return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
  esac
}

# This function attempts to execute a function as the current user and switches to root if it fails.
maybe_root() {
  if ! "$@" >/dev/null 2>&1; then
    if ! as_root "$@"; then
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  fi
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This function verifies if one of the supported download tools is installed and returns with
# $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET if that is not th ecase.
download_tool_exists() {
    log_debug "Determining if a supported download tool is installed..."
    if command_exists "wget"; then
      log_debug "wget is installed."
      return "${TOFU_INSTALL_EXIT_CODE_OK}"
    elif command_exists "curl"; then
      log_debug "curl is installed."
      return "${TOFU_INSTALL_EXIT_CODE_OK}"
    else
      log_debug "No supported download tool is installed."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
    fi
}

# This function downloads the URL specified in $1 into the file specified in $2.
# It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET if no supported download tool is installed, or $TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED
# if the download failed.
download_file() {
  if [ -z "$1" ]; then
    log_error "Bug: no URL supplied to download_file()"
    return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
  fi
  if [ -z "$2" ]; then
    log_error "Bug: no destination file supplied to download_file()"
    return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
  fi
  log_debug "Downloading URL ${1} to ${2}..."
  IS_GITHUB=0
  if [ -n "${GITHUB_TOKEN}" ]; then
    if [ "$(echo "$1" | grep -c "api.github.com" || true)" -ne 0 ]; then
      IS_GITHUB=1
    fi
  fi
  if command_exists "wget"; then
    if [ "${IS_GITHUB}" -eq 1 ]; then
      log_debug "Downloading using wget with GITHUB_TOKEN..."
      if ! wget -q --header="Authorization: token ${GITHUB_TOKEN}" -O "$2" "$1"; then
        log_debug "Download failed."
        return "${TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED}"
      fi
    else
      log_debug "Downloading using wget without GITHUB_TOKEN, this may lead to rate limit issues..."
      if ! wget -q -O "$2" "$1"; then
        log_debug "Download failed, please try specifying the GITHUB_TOKEN environment variable."
        return "${TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED}"
      fi
    fi
  elif command_exists "curl"; then
    if [ "${IS_GITHUB}" -eq 1 ]; then
      log_debug "Downloading using curl with GITHUB_TOKEN..."
      if ! curl --proto '=https' --tlsv1.2 -fsSL -H "Authorization: token ${GITHUB_TOKEN}" -o "$2" "$1"; then
        log_debug "Download failed."
        return "${TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED}"
      fi
    else
      log_debug "Downloading using curl without GITHUB_TOKEN, this may lead to rate limit issues..."
      if ! curl --proto '=https' --tlsv1.2 -fsSL -o "$2" "$1"; then
        log_debug "Download failed, please try specifying the GITHUB_TOKEN environment variable."
        return "${TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED}"
      fi
    fi
  else
    log_error "Neither wget nor curl are available on your system. Please install one of them to proceed."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  log_debug "Download successful."
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This function downloads the OpenTofu GPG key from the specified URL to the specified location. Setting the third
# parameter to 1 causes the file to be moved as root. It returns $TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED if the
# download fails, or $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET if no download tool is available.
download_gpg() {
  if [ -z "$1" ]; then
    log_error "Bug: no URL passed to download_gpg."
    return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
  fi
  if [ -z "$2" ]; then
    log_error "Bug: no destination passed to download_gpg."
    return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
  fi
  if ! command_exists "gpg"; then
    log_error "Missing gpg binary."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  log_debug "Downloading GPG key from ${1} to ${2}..."
  if ! download_tool_exists; then
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  log_debug "Creating temporary directory..."
  TEMPDIR=$(mktemp -d)
  if [ -z "${TEMPDIR}" ]; then
    log_error "Failed to create temporary directory for GPG download."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  TEMPFILE="${TEMPDIR}/opentofu.gpg"

  if ! download_file "${1}" "${TEMPFILE}"; then
    log_debug "Removing temporary directory..."
    rm -rf "${TEMPFILE}"
    return "${TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED}"
  fi
  if [ "$(grep 'BEGIN PGP PUBLIC KEY BLOCK' -c "${TEMPFILE}" || true)" -ne 0 ]; then
    log_debug "Performing GPG dearmor on ${TEMPFILE}"
    if ! gpg --no-tty --batch --dearmor -o "${TEMPFILE}.tmp" <"${TEMPFILE}"; then
      log_error "Failed to GPG dearmor ${TEMPFILE}."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
    if ! mv "${TEMPFILE}.tmp" "${TEMPFILE}"; then
      log_error "Failed to move ${TEMPFILE}.tmp to ${TEMPFILE}."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  fi
  if [ "$3" = "1" ]; then
    log_debug "Moving GPG file as root..."
    if ! as_root mv "${TEMPFILE}" "${2}"; then
      log_error "Failed to move ${TEMPFILE} to ${2}."
      rm -rf "${TEMPFILE}"
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  else
    log_debug "Moving GPG file as the current user..."
    if ! mv "${TEMPFILE}" "${2}"; then
      log_error "Failed to move ${TEMPFILE} to ${2}."
      rm -rf "${TEMPFILE}"
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  fi

  log_debug "Removing temporary directory..."
  rm -rf "${TEMPFILE}"
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This is a helper function that downloads a GPG URL to the specified file.
deb_download_gpg() {
  DEB_GPG_URL="${1}"
  GPG_FILE="${2}"
  if [ -z "${DEB_GPG_URL}" ]; then
    log_error "Bug: no GPG URL specified for deb_download_gpg."
    return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
  fi
  if [ -z "${GPG_FILE}" ]; then
    log_error "Bug: no destination path specified for deb_download_gpg."
    return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
  fi
  if ! download_gpg "${DEB_GPG_URL}" "${GPG_FILE}" 1; then
    log_error "Failed to download GPG key from ${DEB_GPG_URL}."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  log_debug "Changing ownership and permissions of ${GPG_FILE}..."
  if ! as_root chown root:root "${GPG_FILE}"; then
    log_error "Failed to chown ${GPG_FILE}."
    rm -rf "${GPG_FILE}"
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if ! as_root chmod a+r "${GPG_FILE}"; then
    log_error "Failed to chmod ${GPG_FILE}."
    rm -rf "${GPG_FILE}"
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This function installs OpenTofu via a Debian repository. It returns
# $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET if this is not a Debian system.
install_deb() {
  log_info "Attempting installation via Debian repository..."
  if ! command_exists apt-get; then
    log_info "The apt-get command is not available, skipping Debian repository installation."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi

  if ! is_root; then
    log_info "Root privileges are required to install OpenTofu as a Debian package."
    log_info "The installer will now verify if it can correctly assume root privileges."
    log_info "${bold}You may be asked to enter your password.${normal}"
    if ! as_root echo -n ""; then
      log_error "Cannot assume root privileges."
      log_info "Please set up either '${bold}su${normal}' or '${bold}sudo${normal}'."
      log_info "Alternatively, run this script with ${bold}-h${normal} for other installation methods."
    fi
  fi

  log_info "Updating package list..."
  if ! as_root apt-get update; then
    log_error "Failed to update apt package list."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi

  log_debug "Determining packages to install..."
  PACKAGE_LIST="apt-transport-https ca-certificates"
  if [ "${SKIP_VERIFY}" -ne "1" ]; then
    PACKAGE_LIST="${PACKAGE_LIST} gnupg"
  fi
  if ! download_tool_exists; then
    log_debug "No download tool present, adding curl to the package list..."
    PACKAGE_LIST="${PACKAGE_LIST} curl"
  fi

  log_info "Installing necessary packages for installation..."
  log_debug "Installing ${PACKAGE_LIST}..."
  # shellcheck disable=SC2086
  if ! as_root apt-get install -y ${PACKAGE_LIST}; then
    log_error "Failed to install requisite packages for Debian repository installation."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  log_debug "Necessary packages installed."

  if [ "${SKIP_VERIFY}" -ne "1" ]; then
    log_info "Installing the OpenTofu GPG keys..."
    log_debug "Creating /etc/apt/keyrings..."
    if ! as_root install -m 0755 -d /etc/apt/keyrings; then
      log_error "Failed to create /etc/apt/keyrings."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
    log_debug "Created /etc/apt/keyrings."

    PACKAGE_GPG_FILE=/etc/apt/keyrings/opentofu.gpg
    log_debug "Downloading the GPG key from ${DEB_GPG_URL}.."
    if ! deb_download_gpg "${DEB_GPG_URL}" "${PACKAGE_GPG_FILE}"; then
      log_error "Failed to download GPG key from ${DEB_GPG_URL}."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
    if [ -n "${DEB_REPO_GPG_URL}" ] && [ "${DEB_REPO_GPG_URL}" != "-" ]; then
      log_debug "Downloading the repo GPG key from ${DEB_REPO_GPG_URL}.."
      REPO_GPG_FILE=/etc/apt/keyrings/opentofu-repo.gpg
      if ! deb_download_gpg "${DEB_REPO_GPG_URL}" "${REPO_GPG_FILE}" 1; then
        log_error "Failed to download GPG key from ${DEB_REPO_GPG_URL}."
        return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
      fi
    fi
  fi

  log_info "Creating OpenTofu sources list..."
  if [ "${SKIP_VERIFY}" -ne "1" ]; then
    if [ -n "${REPO_GPG_FILE}" ]; then
      if ! as_root tee /etc/apt/sources.list.d/opentofu.list; then
        log_error "Failed to create /etc/apt/sources.list.d/opentofu.list."
        return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
      fi <<EOF
deb [signed-by=${PACKAGE_GPG_FILE},${REPO_GPG_FILE}] ${DEB_REPO_URL} ${DEB_REPO_SUITE} ${DEB_REPO_COMPONENTS}
deb-src [signed-by=${PACKAGE_GPG_FILE},${REPO_GPG_FILE}] ${DEB_REPO_URL} ${DEB_REPO_SUITE} ${DEB_REPO_COMPONENTS}
EOF
    else
      if ! as_root tee /etc/apt/sources.list.d/opentofu.list; then
        log_error "Failed to create /etc/apt/sources.list.d/opentofu.list."
        return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
      fi <<EOF
deb [signed-by=${PACKAGE_GPG_FILE}] ${DEB_REPO_URL} ${DEB_REPO_SUITE} ${DEB_REPO_COMPONENTS}
deb-src [signed-by=${PACKAGE_GPG_FILE}] ${DEB_REPO_URL} ${DEB_REPO_SUITE} ${DEB_REPO_COMPONENTS}
EOF
    fi
  else
    if ! as_root tee /etc/apt/sources.list.d/opentofu.list; then
      log_error "Failed to create /etc/apt/sources.list.d/opentofu.list."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi <<EOF
deb [trusted] ${DEB_REPO_URL} ${DEB_REPO_SUITE} ${DEB_REPO_COMPONENTS}
deb-src [trusted] ${DEB_REPO_URL} ${DEB_REPO_SUITE} ${DEB_REPO_COMPONENTS}
EOF
  fi

  log_info "Updating package list..."
  if ! as_root apt-get update; then
    log_error "Failed to update apt package list."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  log_info "Installing OpenTofu..."
  if ! as_root apt-get install -y tofu; then
    log_error "Failed to install opentofu."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi

  log_info "Checking if OpenTofu is installed correctly..."
  if ! tofu --version > /dev/null; then
    log_error "Failed to run tofu after installation."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This function installs OpenTofu via the zypper command line utility. It returns
# $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET if zypper is not available.
install_zypper() {
  if ! command_exists "zypper"; then
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  log_info "Installing OpenTofu using zypper..."
  if [ "${SKIP_VERIFY}" -ne "1" ]; then
    GPGCHECK=1
    GPG_URL="${RPM_GPG_URL}"
    if [ "${RPM_REPO_GPG_URL}" != "-" ]; then
      GPG_URL=$(cat <<EOF
${GPG_URL}
       ${RPM_REPO_GPG_URL}
EOF
)
    fi
  else
    GPGCHECK=0
    GPG_URL=""
  fi
  if ! as_root tee /etc/zypp/repos.d/opentofu.repo; then
    log_error "Failed to write /etc/zypp/repos.d/opentofu.repo"
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi <<EOF
[opentofu]
name=opentofu
baseurl=${RPM_REPO_URL}\$basearch
repo_gpgcheck=${GPGCHECK}
gpgcheck=${GPGCHECK}
enabled=1
gpgkey=${GPG_URL}
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300

[opentofu-source]
name=opentofu-source
baseurl=${RPM_REPO_URL}SRPMS
repo_gpgcheck=${GPGCHECK}
gpgcheck=${GPGCHECK}
enabled=1
gpgkey=${GPG_URL}
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300
EOF
  for GPG_SRC in ${GPG_URL}; do
    log_debug "Importing GPG key from ${GPG_SRC}..."
    if ! rpm --import "${GPG_SRC}"; then
      log_error "Failed to import GPG key from ${GPG_SRC}."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  done
  for REPO in opentofu opentofu-source; do
    log_debug "Importing GPG key for repo ${REPO} into zypper..."
    if ! as_root zypper --gpg-auto-import-keys refresh opentofu; then
      log_error "Failed to auto-import GPG key for repo ${REPO} into zypper."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  done
  if ! as_root zypper install -y tofu; then
    log_error "Failed to install tofu via zypper."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This function installs OpenTofu via the yum command line utility. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET
# if yum is not available.
install_yum() {
  if ! command_exists "yum"; then
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  log_info "Installing OpenTofu using yum..."
  if [ "${SKIP_VERIFY}" -ne "1" ]; then
    GPGCHECK=1
    GPG_URL="${RPM_GPG_URL}"
    if [ "${RPM_REPO_GPG_URL}" != "-" ]; then
      GPG_URL=$(cat <<EOF
${GPG_URL}
       ${RPM_REPO_GPG_URL}
EOF
)
    fi
  else
    GPGCHECK=0
    GPG_URL=""
  fi
  if ! as_root tee /etc/yum.repos.d/opentofu.repo; then
    log_error "Failed to write /etc/yum.repos.d/opentofu.repo"
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi <<EOF
[opentofu]
name=opentofu
baseurl=${RPM_REPO_URL}\$basearch
repo_gpgcheck=${GPGCHECK}
gpgcheck=${GPGCHECK}
enabled=1
gpgkey=${GPG_URL}
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300

[opentofu-source]
name=opentofu-source
baseurl=${RPM_REPO_URL}SRPMS
repo_gpgcheck=${GPGCHECK}
gpgcheck=${GPGCHECK}
enabled=1
gpgkey=${GPG_URL}
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300
EOF
  for GPG_SRC in ${GPG_URL}; do
    log_debug "Importing GPG key from ${GPG_SRC}..."
    if ! rpm --import "${GPG_SRC}"; then
      log_error "Failed to import GPG key from ${GPG_SRC}."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  done
  if ! as_root yum install -y tofu; then
    log_error "Failed to install tofu via yum."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This function installs OpenTofu via an RPM repository. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET
# if this is not an RPM-based system.
install_rpm() {
  if command_exists "zypper"; then
    install_zypper
    return $?
  else
    install_yum
    return $?
  fi
}

# This function installs OpenTofu via an APK (Alpine Linux) package. It returns
# $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET if this is not an Alpine Linux system.
install_apk() {
  if ! command_exists "apk"; then
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  log_info "Installing OpenTofu using APK..."
  if [ "${APK_REPO_URL}" != "-" ]; then
    APK_REPO_PARAM="--repository=${APK_REPO_URL}"
  else
    APK_REPO_PARAM=""
  fi
  if ! apk add "${APK_PACKAGE}" "${APK_REPO_PARAM}"; then
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This function installs OpenTofu via Snapcraft. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET if
# Snap is not available.
install_snap() {
  if ! command_exists "snap"; then
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  log_info "Installing OpenTofu using Snap..."
  if ! as_root snap install --classic opentofu; then
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This function installs OpenTofu via Homebrew. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET if
# Homebrew is not available.
install_brew() {
  if ! command_exists "brew"; then
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  log_info "Installing OpenTofu using Homebrew..."
  log_info "Updating brew..."
  if ! brew update; then
    log_info "brew update failed."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if ! brew install opentofu; then
    log_info "brew install opentofu failed."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

# This function installs OpenTofu as a standalone installation. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED if the installation
# was unsuccessful.
install_standalone() {
  if ! download_tool_exists; then
    log_error "Neither wget nor curl are available on your system. Please install at least one of them to proceed."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  if ! command_exists "unzip"; then
    log_warning "Unzip is missing, please install it to use the standalone installation method."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
  fi
  if ! command_exists "shasum"; then
    if ! command_exists "sha256sum"; then
      log_warning "shasum is missing, please install it to use the standalone installation method."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
    fi
  fi

  if [ "${SKIP_VERIFY}" -ne "1" ]; then
    if ! command_exists "${COSIGN_PATH}"; then
      log_error "Cosign is not installed on your system, which is required to verify package integrity."
      log_info "If you have cosign installed, please pass the --cosign-path option. Alternatively, you can disable integrity verification by passing ${bold}--skip-verify${normal} (not recommended)."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}"
    fi
  fi

  log_info "Installing OpenTofu using the standalone installation method..."

  if [ "${OPENTOFU_VERSION}" = "latest" ]; then
    log_info "Determining latest OpenTofu version..."
    OPENTOFU_VERSION=$(download_file "https://api.github.com/repos/opentofu/opentofu/releases/latest" - | grep "tag_name" | sed -e 's/.*tag_name": "v//' -e 's/".*//')
    if [ -z "${OPENTOFU_VERSION}" ]; then
      log_error "Failed to obtain latest release from the GitHub API. Try passing --opentofu-version to specify a version."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  fi
  OS="$(uname | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m | sed -e 's/aarch64/arm64/' -e 's/x86_64/amd64/')"
  if [ -z "${OS}" ]; then
    log_error "Failed to determine OS version."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if [ -z "${ARCH}" ]; then
    log_error "Failed to determine CPU architecture."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi

  log_info "Downloading OpenTofu version ${OPENTOFU_VERSION}..."
  TEMPDIR="$(mktemp -d)"
  if [ -z "${TEMPDIR}" ]; then
    log_error "Failed to create temporary directory"
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  # shellcheck disable=SC2064
  trap "rm -rf '${TEMPDIR}' || true" EXIT

  ZIPDIR="$(mktemp -d)"
  if [ -z "${ZIPDIR}" ]; then
    log_error "Failed to create temporary directory"
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  # shellcheck disable=SC2064
  trap "rm -rf '${ZIPDIR}' || true" EXIT

  ZIPFILE="tofu_${OPENTOFU_VERSION}_${OS}_${ARCH}.zip"
  if ! download_file "https://github.com/opentofu/opentofu/releases/download/v${OPENTOFU_VERSION}/${ZIPFILE}" "${TEMPDIR}/${ZIPFILE}"; then
    log_error "Failed to download the OpenTofu release ${OPENTOFU_VERSION}."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi

  log_info "Performing checksum verification..."
  SUMSFILE="tofu_${OPENTOFU_VERSION}_SHA256SUMS"
  if ! download_file "https://github.com/opentofu/opentofu/releases/download/v${OPENTOFU_VERSION}/${SUMSFILE}" "${TEMPDIR}/${SUMSFILE}"; then
    log_error "Failed to download ${SUMSFILE} for OpenTofu version ${OPENTOFU_VERSION}."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if command_exists "shasum"; then
    REALSUM=$(shasum -a 256 "${TEMPDIR}/${ZIPFILE}" | cut -f 1 -d ' ')
  else
    REALSUM=$(sha256sum "${TEMPDIR}/${ZIPFILE}" | cut -f 1 -d ' ')
  fi
  if [ -z "${REALSUM}" ]; then
    log_error "Checksum could not be calculated for ${TEMPDIR}/${ZIPFILE}."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  EXPECTEDSUM=$(grep "${ZIPFILE}" "${TEMPDIR}/${SUMSFILE}" | cut -f 1 -d ' ')
  if [ -z "${EXPECTEDSUM}" ]; then
    log_error "No checksum found for ${ZIPFILE}."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  if [ "${REALSUM}" != "${EXPECTEDSUM}" ]; then
    log_error "Checksum mismatch for ${ZIPFILE}, expected: ${EXPECTEDSUM}, got: ${REALSUM}."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi
  log_info "Checksum for ${ZIPFILE} is ${REALSUM}, as expected."

  if [ "${SKIP_VERIFY}" -ne "1" ]; then
    log_info "Performing signature verification..."
    SIGFILE="tofu_${OPENTOFU_VERSION}_SHA256SUMS.sig"
    CERTFILE="tofu_${OPENTOFU_VERSION}_SHA256SUMS.pem"
    for FILE in "${SIGFILE}" "${CERTFILE}"; do
      if ! download_file "https://github.com/opentofu/opentofu/releases/download/v${OPENTOFU_VERSION}/${FILE}" "${TEMPDIR}/${FILE}"; then
        log_error "Failed to download ${FILE} for OpenTofu version ${OPENTOFU_VERSION}."
        return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
      fi
    done

    IDENTITY="${COSIGN_IDENTITY}"
    if [ "${IDENTITY}" = "autodetect" ]; then
      if [ "${OPENTOFU_VERSION}" = "1.6.0-beta4" ] || \
         [ "${OPENTOFU_VERSION}" = "1.6.0-beta3" ] || \
         [ "${OPENTOFU_VERSION}" = "1.6.0-beta2" ] || \
         [ "${OPENTOFU_VERSION}" = "1.6.0-beta1" ] || \
         [ "${OPENTOFU_VERSION}" = "1.6.0-alpha5" ] || \
         [ "${OPENTOFU_VERSION}" = "1.6.0-alpha4" ] || \
         [ "${OPENTOFU_VERSION}" = "1.6.0-alpha3" ] || \
         [ "${OPENTOFU_VERSION}" = "1.6.0-alpha2" ] || \
         [ "${OPENTOFU_VERSION}" = "1.6.0-alpha1" ]; then
          IDENTITY="https://github.com/opentofu/opentofu/.github/workflows/release.yml@refs/tags/v${OPENTOFU_VERSION}"
      else
        if [ "$(echo "${OPENTOFU_VERSION}" | grep -c "alpha" || true)" -ne "0" ] || [ "$(echo "${OPENTOFU_VERSION}" | grep -c "beta" || true)" -ne "0" ]; then
          IDENTITY="https://github.com/opentofu/opentofu/.github/workflows/release.yml@refs/heads/main"
        else
          IDENTITY="https://github.com/opentofu/opentofu/.github/workflows/release.yml@refs/heads/v$(echo "${OPENTOFU_VERSION}" | sed -e "s/\([0-9]*\)\.\([0-9]*\)\..*/\1.\2/")"
        fi
      fi
    fi

    if ! "${COSIGN_PATH}" verify-blob \
      --certificate-identity "${IDENTITY}" \
      --signature "${TEMPDIR}/${SIGFILE}" \
      --certificate "${TEMPDIR}/${CERTFILE}" \
      --certificate-oidc-issuer "${COSIGN_OIDC_ISSUER}" \
      "${TEMPDIR}/${SUMSFILE}"; then
        log_error "Signature verification failed, the downloaded files may be corrupted."
        return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
    log_info "Verification successful."
  fi

  log_info "Unpacking OpenTofu..."
  if ! unzip -d "${ZIPDIR}" "${TEMPDIR}/${ZIPFILE}"; then
    log_error "Failed to unzip ${TEMPDIR}/${ZIPFILE} to /"
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi

  log_info "Moving OpenTofu installation to ${INSTALL_PATH}..."
  if ! maybe_root mkdir -p "${INSTALL_PATH}"; then
    log_error "Cannot create installation path at ${INSTALL_PATH}."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi

  if ! maybe_root mv "${ZIPDIR}"/* "${INSTALL_PATH}" >/dev/null 2>&1; then
    log_error "Cannot move ${ZIPDIR} contents to ${INSTALL_PATH}. Please check the permissions on the target directory."
    return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
  fi

  if [ "${SYMLINK_PATH}" != "-" ]; then
    log_info "Creating tofu symlink at ${SYMLINK_PATH}/tofu..."
    if ! maybe_root ln -sf "${INSTALL_PATH}/tofu" "${SYMLINK_PATH}/tofu"; then
      log_error "Failed to create symlink at ${INSTALL_PATH}/tofu."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  fi
  log_info "Checking if OpenTofu is installed correctly..."
  if [ "${SYMLINK_PATH}" != "-" ]; then
    if ! "${SYMLINK_PATH}/tofu" --version; then
      log_error "Failed to run ${SYMLINK_PATH}/tofu after installation."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  else
    if ! "${INSTALL_PATH}/tofu" --version; then
      log_error "Failed to run ${INSTALL_PATH}/tofu after installation."
      return "${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}"
    fi
  fi
  log_success "Installation complete."
  return "${TOFU_INSTALL_EXIT_CODE_OK}"
}

usage() {
  if [ -n "$1" ]; then
    log_error "Error: $1"
  fi
  cat <<EOF
${bold}${blue}Usage:${normal} $(basename "$0") ${magenta}[OPTIONS]${normal}

${bold}${blue}OPTIONS for all installation methods:${normal}

  ${bold}-h|--help${normal}                     Print this help.
  ${bold}--root-method ${magenta}METHOD${normal}          The method to use to obtain root credentials.
                                (${bold}One of:${normal} ${magenta}none${normal}, ${magenta}su${normal}, ${magenta}sudo${normal}, ${magenta}auto${normal}; ${bold}default:${normal} ${magenta}auto${normal})
  ${bold}--install-method ${magenta}METHOD${normal}       The installation method to use. (${red}required${normal})
                                Must be one of:
                                    ${magenta}deb${normal}         Debian repository installation
                                    ${magenta}rpm${normal}         RPM repository installation
                                    ${magenta}apk${normal}         APK (Alpine) repository installation
                                    ${magenta}snap${normal}        Snapcraft installation
                                    ${magenta}brew${normal}        Homebrew installation
                                    ${magenta}standalone${normal}  Standalone installation
  ${bold}--skip-verify${normal}                 Skip GPG or cosign integrity verification.
                                (${bold}${red}not recommended${normal}).
  ${bold}--debug${normal}                       Enable debug logging.

${bold}${blue}OPTIONS for the Debian repository installation:${normal}

  ${bold}--deb-url ${magenta}URL${normal}                 Debian repository URL.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_DEB_REPO_URL}${normal})
  ${bold}--deb-suite ${magenta}SUITE${normal}             Debian repository suite.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_DEB_REPO_SUITE}${normal})
  ${bold}--deb-components ${magenta}COMPONENTS${normal}   Debian repository components.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_DEB_REPO_COMPONENTS}${normal})
  ${bold}--deb-gpg-url ${magenta}URL${normal}             The URL where the GPG signing key is located.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_DEB_GPG_URL}${normal})
  ${bold}--deb-repo-gpg-url ${magenta}URL${normal}        Sets the GPG key for the Debian repository.
                                This is a workaround and may be removed in the future.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_DEB_REPO_GPG_URL}${normal}}

${bold}${blue}OPTIONS for the RPM repository installation:${normal}

  ${bold}--rpm-url ${magenta}URL${normal}                 RPM repository URL.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_RPM_REPO_URL}${normal})
  ${bold}--rpm-gpg-url ${magenta}URL${normal}             The URL where the GPG signing key is located.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_RPM_GPG_URL}${normal})
  ${bold}--rpm-repo-gpg-url ${magenta}URL${normal}        Sets the GPG key for the RPM repository.
                                This is a workaround and may be removed in the future.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_RPM_REPO_GPG_URL}${normal}}

${bold}${blue}OPTIONS for the Alpine repository installation:${normal}

  ${bold}--apk-repo ${magenta}URL${normal}                APK repository URL. Pass ${bold}-${normal} to install from
                                the included packages.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_APK_REPO_URL}${normal})
  ${bold}--apk-package ${magenta}PACKAGE${normal}         APK package to install.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_APK_PACKAGE}${normal})

${bold}${blue}OPTIONS for the standalone installation:${normal}

  ${bold}--opentofu-version ${magenta}VERSION${normal}    Installs the specified OpenTofu version.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_OPENTOFU_VERSION}${normal})
  ${bold}--install-path ${magenta}PATH${normal}           Installs OpenTofu to the specified path.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_INSTALL_PATH}${normal})
  ${bold}--symlink-path ${magenta}PATH${normal}           Symlink the OpenTofu binary to this directory.
                                Pass "${bold}-${normal}" to skip creating a symlink.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_SYMLINK_PATH}${normal})
  ${bold}--cosign-path ${magenta}PATH${normal}            Path to cosign. (${bold}Default:${normal} ${magenta}${DEFAULT_COSIGN_PATH}${normal})
  ${bold}--cosign-oidc-issuer ${magenta}ISSUER${normal}   OIDC issuer for cosign verification.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_COSIGN_OIDC_ISSUER}${normal})
  ${bold}--cosign-identity ${magenta}IDENTITY${normal}    Cosign certificate identity.
                                (${bold}Default:${normal} ${magenta}${DEFAULT_COSIGN_IDENTITY}${normal})

  ${bold}Note:${normal} If you do not specify the OpenTofu version, the script calls the GitHub API.
  This API is rate-limited. If you encounter problems, please create a GitHub token at
  https://github.com/settings/tokens without any permissions and set the ${bold}GITHUB_TOKEN${normal}
  environment variable. This will increase the rate limit.

${bold}${blue}Exit codes:${normal}

  ${bold}${TOFU_INSTALL_EXIT_CODE_OK}${normal}                             Installation successful.
  ${bold}${TOFU_INSTALL_EXIT_CODE_INSTALL_REQUIREMENTS_NOT_MET}${normal}                             Your system is missing one or more requirements
                                for these selected installation method. Please
                                install the indicated tools to continue.
                                (e.g. Homebrew for the ${bold}brew${normal} installation method.)
  ${bold}${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}${normal}                             The installation failed.
  ${bold}${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}${normal}                             Invalid configuration options.

EOF
}

main() {
  echo "${blue}${bold}OpenTofu Installer${normal}"
  echo ""

  while [ -n "$1" ]; do
    case $1 in
      -h)
        usage
        return "${TOFU_INSTALL_EXIT_CODE_OK}"
        ;;
      --help)
        usage
        return "${TOFU_INSTALL_EXIT_CODE_OK}"
        ;;
      --root-method)
        shift
        case $1 in
          auto|sudo|su|none)
            ROOT_METHOD=$1
            ;;
          "")
            usage "--root-method requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
          *)
            if [ -z "$1" ]; then
              usage "Invalid value for --root-method: $1."
              return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            fi
        esac
        ;;
      --install-method)
        shift
        case $1 in
          auto|deb|rpm|apk|snap|brew|standalone)
            INSTALL_METHOD=$1
            ;;
          "")
            usage "--install-method requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
          *)
            if [ -z "$1" ]; then
              usage "Invalid value for --install-method: $1."
              return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            fi
        esac
        ;;
      --install-path)
        shift
        case $1 in
          "")
            usage "--install-path requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
          *)
            INSTALL_PATH=$1
            ;;
        esac
        ;;
      --symlink-path)
        shift
        case $1 in
          "")
            usage "--symlink-path requires an argument (pass - to skip creating a symlink)."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
          *)
            SYMLINK_PATH=$1
            ;;
        esac
        ;;
      --deb-gpg-url)
        shift
        case $1 in
          "")
            usage "--deb-gpg-url requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            DEB_GPG_URL="${1}"
            ;;
        esac
        ;;
      --deb-repo-gpg-url)
        shift
        case $1 in
          "")
            usage "--deb-repo-gpg-url requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            DEB_REPO_GPG_URL="${1}"
            ;;
        esac
        ;;
      --deb-url)
        shift
        case $1 in
          "")
            usage "--deb-url requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            DEB_REPO_URL="${1}"
            ;;
        esac
        ;;
      --deb-suite)
        shift
        case $1 in
          "")
            usage "--deb-suite requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            DEB_REPO_SUITE="${1}"
            ;;
        esac
        ;;
      --deb-components)
        shift
        case $1 in
          "")
            usage "--deb-components requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            DEB_REPO_COMPONENTS="${1}"
            ;;
        esac
        ;;
      --rpm-url)
        shift
        case $1 in
          "")
            usage "--rpm-url requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            RPM_REPO_URL="${1}"
            ;;
        esac
        ;;
      --rpm-gpg-url)
        shift
        case $1 in
          "")
            usage "--rpm-gpg-url requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            RPM_GPG_URL="${1}"
            ;;
        esac
        ;;
      --rpm-repo-gpg-url)
        shift
        case $1 in
          "")
            usage "--rpm-repo-gpg-url requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            RPM_REPO_GPG_URL="${1}"
            ;;
        esac
        ;;
      --apk-repo)
        shift
        case $1 in
          "")
            usage "--apk-repo requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            APK_REPO_URL="${1}"
            ;;
        esac
        ;;
      --apk-package)
        shift
        case $1 in
          "")
            usage "--apk-package requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            APK_PACKAGE="${1}"
            ;;
        esac
        ;;
      --cosign-path)
        shift
        case $1 in
          "")
            usage "--cosign-path requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            COSIGN_PATH="${1}"
            ;;
        esac
        ;;
      --cosign-oidc-issuer)
        shift
        case $1 in
          "")
            usage "--cosign-oidc-issuer requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            COSIGN_OIDC_ISSUER="${1}"
            ;;
        esac
        ;;
      --cosign-identity)
        shift
        case $1 in
          "")
            usage "--cosign-identity requires an argument."
            return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
            ;;
        *)
            COSIGN_IDENTITY="${1}"
            ;;
        esac
        ;;
      --skip-verify)
        SKIP_VERIFY=1
        log_warning "Skipping integrity verification. This is not recommended."
        ;;
      --debug)
        LOG_DEBUG=1
        ;;
      *)
        usage "Unknown option: $1."
        return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
        ;;
    esac
    shift
  done
  case "${INSTALL_METHOD}" in
  deb)
    install_deb
    return $?
    ;;
  rpm)
    install_rpm
    return $?
    ;;
  apk)
    install_apk
    return $?
    ;;
  snap)
    install_snap
    return $?
    ;;
  brew)
    install_brew
    return $?
    ;;
  standalone)
    install_standalone
    return $?
    ;;
  "")
    log_error "Please select an installation method with --install-method."
    return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
    ;;
  *)
    log_error "Usupported installation method: ${INSTALL_METHOD}."
    return "${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}"
  esac
}

main "$@"
exit $?