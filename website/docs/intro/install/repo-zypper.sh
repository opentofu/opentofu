cat >/etc/zypp/repos.d/opentofu.repo <<EOF
[opentofu]
name=opentofu
baseurl=https://packagecloud.io/opentofu/tofu/rpm_any/rpm_any/\$basearch
repo_gpgcheck=1
gpgcheck=0
enabled=1
gpgkey=https://packagecloud.io/opentofu/tofu/gpgkey
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300

[opentofu-source]
name=opentofu-source
baseurl=https://packagecloud.io/opentofu/tofu/rpm_any/rpm_any/SRPMS
repo_gpgcheck=1
gpgcheck=0
enabled=1
gpgkey=https://packagecloud.io/opentofu/tofu/gpgkey
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300
EOF