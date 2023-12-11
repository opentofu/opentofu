#!/bin/sh

# This is a combined POSIX/PowerShell script for installing OpenTofu. The Powershell part is below.
#
# Note: do not use #> in the POSIX part.
#
# See https://stackoverflow.com/questions/39421131/is-it-possible-to-write-one-script-that-runs-in-bash-shell-and-powershell

echo --% >/dev/null;: ' | out-null
<#'

# region POSIX

export TOFU_INSTALL_EXIT_CODE_OK=0
export TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED=1
export TOFU_INSTALL_EXIT_CODE_DOWNLOAD_TOOL_MISSING=2
export TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED=3
export TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT=4

TOFU_INSTALL_RETURN_CODE_COMMAND_NOT_FOUND=11
TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED=13

if [ -t 1 ]; then
    colors=$(tput colors)

    if [ "$colors" -ge 8 ]; then
        bold="$(tput bold)"
        underline="$(tput smul)"
        standout="$(tput smso)"
        normal="$(tput sgr0)"
        black="$(tput setaf 0)"
        red="$(tput setaf 1)"
        green="$(tput setaf 2)"
        yellow="$(tput setaf 3)"
        blue="$(tput setaf 4)"
        magenta="$(tput setaf 5)"
        cyan="$(tput setaf 6)"
        white="$(tput setaf 7)"
    fi
fi

ROOT_METHOD=auto
INSTALL_METHOD=auto
DEFAULT_INSTALL_PATH=/opt/opentofu
INSTALL_PATH=$DEFAULT_INSTALL_PATH
DEFAULT_SYMLINK_PATH=/usr/local/bin
SYMLINK_PATH=$DEFAULT_SYMLINK_PATH
DEFAULT_OPENTOFU_VERSION=latest
OPENTOFU_VERSION="${DEFAULT_OPENTOFU_VERSION}"

log_success() {
  if [ -z "$1" ]; then
    return
  fi
  echo "${green}$1${normal}"
}

log_warning() {
  if [ -z "$1" ]; then
    return
  fi
  echo "${yellow}$1${normal}"
}

log_info() {
  if [ -z "$1" ]; then
    return
  fi
  echo "${cyan}$1${normal}"
}

log_error() {
  if [ -z "$1" ]; then
    return
  fi
  echo "${red}$1${normal}"
}

# This function checks if the command specified in $1 exists.
command_exists() {
  if [ -z "$1" ]; then
    log_error "Bug: no command supplied to command_exists()"
    return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
  fi
  if ! command -v "$1" >dev/null 2>&1; then
    return $TOFU_INSTALL_RETURN_CODE_COMMAND_NOT_FOUND
  fi
  return $TOFU_INSTALL_EXIT_CODE_OK
}

# This function runs the specified command as root.
as_root() {
  case "$ROOT_METHOD" in
    auto)
      if [ "$(id -u)" -eq 0 ]; then
        "$@"
      elif command_exists "sudo"; then
        sudo "$@"
      else
        su root "$@"
      fi
      return $?
      ;;
    none)
      "$@"
      return $?
      ;;
    sudo)
      sudo "$@"
      return $?
      ;;
    su)
      su root "$@"
      return $?
      ;;
    *)
      log_error "Bug: invalid root method value: $1"
      return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
  esac
}

# This function verifies if one of the supported download tools is installed and returns with
# $TOFU_INSTALL_EXIT_CODE_DOWNLOAD_TOOL_MISSING if that is not th ecase.
download_tool_exists() {
    if command_exists "wget"; then
      return $TOFU_INSTALL_EXIT_CODE_OK
    elif command_exists "curl"; then
      return $TOFU_INSTALL_EXIT_CODE_OK
    else
      return $TOFU_INSTALL_EXIT_CODE_DOWNLOAD_TOOL_MISSING
    fi
}

# This function downloads the URL specified in $1 into the file specified in $2.
# It returns $TOFU_INSTALL_EXIT_CODE_DOWNLOAD_TOOL_MISSING if no supported download tool is installed, or $TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED
# if the download failed.
download_file() {
  if [ -z "$1" ]; then
    log_error "Bug: no URL supplied to download_file()"
    return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
  fi
  if [ -z "$2" ]; then
    log_error "Bug: no destination file supplied to download_file()"
    return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
  fi
  if command_exists "wget"; then
    if ! wget -q -o "$2" "$1"; then
      return $TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED
    fi
  elif command_exists "curl"; then
    if ! curl -s -o "$2" "$1"; then
      return $TOFU_INSTALL_RETURN_CODE_DOWNLOAD_FAILED
    fi
  else
    log_error "Neither wget nor curl are available on your system. Please install one of them to proceed."
    return $TOFU_INSTALL_EXIT_CODE_DOWNLOAD_TOOL_MISSING
  fi
  return $TOFU_INSTALL_EXIT_CODE_OK
}

# This function installs OpenTofu via a Debian repository. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
# if this is not a Debian system.
install_deb() {
  if ! command_exists apt-get; then
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
  fi

  #TODO once the packages are gpg-signed, add GPG checks here.
  log_warning "The Debian installation method currently does not perform a GPG signature check. This will be changed in future releases of this script when the OpenTofu packages are GPG-signed."
  PACKAGE_LIST=apt-transport-https ca-certificates

  if ! as_root apt-get update; then
    log_error "Failed to update apt package list."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi

  # shellcheck disable=SC2086
  if ! as_root apt-get install -y $PACKAGE_LIST; then
    log_error "Failed to install requisite packages for Debian repository installation."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi

  if ! as_root tee /etc/apt/sources.list.d/opentofu.list > /dev/null; then
    log_error "Failed to create /etc/apt/sources.list.d/opentofu.list."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi <<EOF
deb [trusted=yes] https://packages.opentofu.org/opentofu/tofu/any/ any main
deb-src [trusted=yes] https://packages.opentofu.org/opentofu/tofu/any/ any main
EOF

  if ! tofu --version > /dev/null; then
    log_error "Failed to run tofu after installation."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  return $TOFU_INSTALL_EXIT_CODE_OK
}

# This function installs OpenTofu via the zypper command line utility. It returns
# $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED if zypper is not available.
install_zypper() {
  if ! command_exists "zypper"; then
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
  fi
  #TODO once the packages are gpg-signed, add GPG checks here.
  log_warning "The Zypper installation method currently does not perform a GPG signature check. This will be changed in future releases of this script when the OpenTofu packages are GPG-signed."
  if ! as_root tee /etc/zypp/repos.d/opentofu.repo; then
    log_error "Failed to write /etc/zypp/repos.d/opentofu.repo"
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi <<EOF
[opentofu]
name=opentofu
baseurl=https://packages.opentofu.org/opentofu/tofu/rpm_any/rpm_any/\$basearch
repo_gpgcheck=0
gpgcheck=0
enabled=1
gpgkey=https://packages.opentofu.org/opentofu/tofu/gpgkey
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300

[opentofu-source]
name=opentofu-source
baseurl=https://packages.opentofu.org/opentofu/tofu/rpm_any/rpm_any/SRPMS
repo_gpgcheck=0
gpgcheck=0
enabled=1
gpgkey=https://packages.opentofu.org/opentofu/tofu/gpgkey
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300
EOF
  if ! as_root yum install -y tofu; then
    log_error "Failed to install tofu via yum."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  return $TOFU_INSTALL_EXIT_CODE_OK
}

# This function installs OpenTofu via the yum command line utility. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
# if yum is not available.
install_yum() {
  if ! command_exists "yum"; then
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
  fi
  #TODO once the packages are gpg-signed, add GPG checks here.
  log_warning "The yum installation method currently does not perform a GPG signature check. This will be changed in future releases of this script when the OpenTofu packages are GPG-signed."
  if ! as_root tee /etc/yum.repos.d/opentofu.repo; then
    log_error "Failed to write /etc/yum.repos.d/opentofu.repo"
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi <<EOF
[opentofu]
name=opentofu
baseurl=https://packages.opentofu.org/opentofu/tofu/rpm_any/rpm_any/\$basearch
repo_gpgcheck=0
gpgcheck=0
enabled=1
gpgkey=https://packages.opentofu.org/opentofu/tofu/gpgkey
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300

[opentofu-source]
name=opentofu-source
baseurl=https://packages.opentofu.org/opentofu/tofu/rpm_any/rpm_any/SRPMS
repo_gpgcheck=0
gpgcheck=0
enabled=1
gpgkey=https://packages.opentofu.org/opentofu/tofu/gpgkey
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300
EOF
  if ! as_root yum install -y tofu; then
    log_error "Failed to install tofu via yum."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  return $TOFU_INSTALL_EXIT_CODE_OK
}

# This function installs OpenTofu via an RPM repository. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
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
# $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED if this is not an Alpine Linux system.
install_apk() {
  if ! command_exists "apk"; then
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
  fi
  #TODO once the package makes it into stable, remove the repository.
  log_warning "The apk installation method currently uses the testing repository."
  if ! apk add opentofu --repository=https://dl-cdn.alpinelinux.org/alpine/edge/testing/; then
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  return $TOFU_INSTALL_EXIT_CODE_OK
}

# This function installs OpenTofu via Snapcraft. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED if
# Snap is not available.
install_snap() {
  if ! command_exists "snap"; then
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
  fi
  if ! snap install --classic opentofu; then
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  return $TOFU_INSTALL_EXIT_CODE_OK
}

# This function installs OpenTofu via Homebrew. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED if
# Homebrew is not available.
install_brew() {
  if ! command_exists "brew"; then
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
  fi
  if ! brew install opentofu; then
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  if ! tofu --version; then
    log_error "Failed to run tofu after installation."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  return $TOFU_INSTALL_EXIT_CODE_OK
}

# This function installs OpenTofu as a portable installation. It returns $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED if the installation
# was unsuccessful.
install_portable() {
  if ! download_tool_exists; then
    log_error "Neither wget nor curl are available on your system. Please install at least one of them to proceed."
    return $TOFU_INSTALL_EXIT_CODE_DOWNLOAD_TOOL_MISSING
  fi
  if ! command_exists "unzip"; then
    log_warning "Unzip is missing, please install it to use the portable installation method."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED
  fi
  if [ "$OPENTOFU_VERSION" = "latest" ]; then
    OPENTOFU_VERSION=$(download_file "https://api.github.com/repos/opentofu/opentofu/releases/latest" - | grep "tag_name" | sed -e 's/.*tag_name": "v//' -e 's/".*//')
    if [ -z "$OPENTOFU_VERSION" ]; then
      log_error "Failed to obtain latest release from the GitHub API. Try passing --opentofu-version to specify a version."
      return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
    fi
  fi
  OS="$(uname | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m | sed -e 's/aarch64/arm64/' -e 's/x86_64/amd64/')"

  if ! as_root mkdir -p "$INSTALL_PATH"; then
    log_error "Failed to create installation directory at $INSTALL_PATH"
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  LWD=$(pwd)
  cd "$INSTALL_PATH" || true
  trap 'cd $LWD' EXIT

  TEMPDIR="$(mktemp -d)"
  if [ -z "$TEMPDIR" ]; then
    cd "$LWD" || true
    log_error "Failed to create temporary directory"
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi

  TEMPFILE="${TEMPDIR}/opentofu.zip)"
  if ! download_file "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_${OS}_${ARCH}.zip" "$TEMPFILE"; then
    cd "$LWD" || true
    rm -rf "$TEMPDIR" || true
    log_error "Failed to download the OpenTofu release $TOFU_VERSION."
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi
  if ! as_root unzip "$TEMPFILE}"; then
    log_error "Failed to unzip $TEMPFILE to /"
    return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
  fi

  if [ "${SYMLINK_PATH}" != "-" ]; then
    if ! as_root ln -s "${INSTALL_PATH}/tofu" "${SYMLINK_PATH}/tofu"; then
      log_error "Failed to create symlink at ${INSTALL_PATH}/tofu."
      return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
    fi
    if ! "${SYMLINK_PATH}/tofu" --version; then
      log_error "Failed to run ${SYMLINK_PATH}/tofu after installation."
      return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
    fi
  else
    if ! "${INSTALL_PATH}/tofu" --version; then
      log_error "Failed to run ${INSTALL_PATH}/tofu after installation."
      return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
    fi
  fi

  return $TOFU_INSTALL_EXIT_CODE_OK
}

# This function iterates through all installation methods and attempts to execute them. If the method is not supported
# it moves on to the next one.
install() {
  for install_func in install_deb install_rpm install_apk install_snap install_brew install_portable; do
    $install_func
    RETURN_CODE=$?
    if [ $RETURN_CODE -eq $TOFU_INSTALL_EXIT_CODE_OK ]; then
      return $TOFU_INSTALL_EXIT_CODE_OK
    elif [ $RETURN_CODE -ne $TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED ]; then
      return $RETURN_CODE
    fi
  done
  log_error "No viable installation method found."
  return $TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED
}

usage() {
  echo "${bold}${blue}OpenTofu installer${normal}"
  echo ""
  if [ -n "$1" ]; then
    log_error "Error: $1"
  fi
  cat <<EOF
${bold}${blue}Usage:${normal} install.sh ${magenta}[OPTIONS]${normal}

${bold}${blue}OPTIONS for all installation methods:${normal}

  ${bold}-h|--help${normal}                     Print this help.

  ${bold}--root-method ${magenta}[METHOD]${normal}        The method to use to obtain root credentials
                                (One of: none, su, sudo, auto; default: auto)

  ${bold}--install-method ${magenta}[METHOD]${normal}     The installation method to use. Must be one of:

                                  auto      Automatically select installation
                                            method (default)
                                  deb       Debian repository installation
                                  rpm       RPM repository installation
                                  apk       APK (Alpine) repository installation
                                  snap      Snapcraft installation
                                  brew      Homebrew installation
                                  portable  Portable installation

${bold}${blue}OPTIONS for the portable installation:${normal}

  ${bold}--opentofu-version ${magenta}[VERSION]${normal}  Installs the specified OpenTofu version.
                                (Default: ${bold}${DEFAULT_OPENTOFU_VERSION}${normal})
  ${bold}--install-path ${magenta}[PATH]${normal}         Installs OpenTofu to the specified path.
                                (Default: ${bold}${DEFAULT_INSTALL_PATH}${normal})

  ${bold}--symlink-path ${magenta}[PATH]${normal}         Symlink the OpenTofu binary to this directory.
                                Pass "${bold}-${normal}" to skip creating a symlink.
                                (Default: ${bold}${DEFAULT_SYMLINK_PATH}${normal})

${bold}${blue}Exit codes:${normal}

  ${bold}${TOFU_INSTALL_EXIT_CODE_OK}${normal}                             Installation successful.
  ${bold}${TOFU_INSTALL_EXIT_CODE_INSTALL_METHOD_NOT_SUPPORTED}${normal}                             The selected installation method is not
                                supported.
  ${bold}${TOFU_INSTALL_EXIT_CODE_DOWNLOAD_TOOL_MISSING}${normal}                             You must install either ${bold}curl${normal} or ${bold}wget${normal}.
  ${bold}${TOFU_INSTALL_EXIT_CODE_INSTALL_FAILED}${normal}                             The installation failed.
  ${bold}${TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT}${normal}                             Invalid configuration options.
EOF
}

main() {
  while [ -n "$1" ]; do
    case $1 in
      -h)
        usage
        return $TOFU_INSTALL_EXIT_CODE_OK
        ;;
      --help)
        usage
        return $TOFU_INSTALL_EXIT_CODE_OK
        ;;
      --root-method)
        shift
        case $1 in
          auto|sudo|su|none)
            ROOT_METHOD=$1
            ;;
          "")
            usage "--root-method requires an argument."
            return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
            ;;
          *)
            if [ -z "$1" ]; then
              usage "Invalid value for --root-method: $1."
              return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
            fi
        esac
        ;;
      --install-method)
        shift
        case $1 in
          auto|deb|rpm|apk|snap|brew|portable)
            INSTALL_METHOD=$1
            ;;
          "")
            usage "--install-method requires an argument."
            return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
            ;;
          *)
            if [ -z "$1" ]; then
              usage "Invalid value for --install-method: $1."
              return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
            fi
        esac
        ;;
      --install-path)
        shift
        case $1 in
          "")
            usage "--install-path requires an argument."
            return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
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
            return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
            ;;
          *)
            SYMLINK_PATH=$1
            ;;
        esac
        ;;
      *)
        usage "Unknown option: $1."
        return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
        ;;
    esac
    shift
  done
  case $INSTALL_METHOD in
  auto)
    install
    return $?
    ;;
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
  portable)
    install_portable
    return $?
    ;;
  *)
    log_error "Bug: unsupported installation method: ${INSTALL_METHOD}."
    return $TOFU_INSTALL_EXIT_CODE_INVALID_ARGUMENT
  esac
}

main "$@"
# This exit is important! It guards the Powershell part below!
exit $?

# endregion

#>

# region Powershell

# endregion