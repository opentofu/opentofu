module github.com/opentofu/opentofu

// At the time of adding this configuration, the new Go feature introduced here https://github.com/golang/go/issues/67061,
// was having a good amount of issues linked to, affecting AWS Firewall, GCP various services and a lot more.
// In go1.23 the godebug flag for this was named 'tlskyber', renamed in go1.24 to 'tlsmlkem'. https://tip.golang.org/doc/godebug#go-124
godebug (
	tlsmlkem=0
	winsymlink=0 // See https://github.com/opentofu/opentofu/pull/3289
)

require (
	cloud.google.com/go/kms v1.15.5
	cloud.google.com/go/storage v1.36.0
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.19.0
	github.com/Netflix/go-expect v0.0.0-20220104043353-73e0943537d2
	github.com/ProtonMail/go-crypto v0.0.0-20230619160724-3fbb1f12458c
	github.com/agext/levenshtein v1.2.3
	github.com/aliyun/alibaba-cloud-sdk-go v1.61.1501
	github.com/aliyun/aliyun-oss-go-sdk v2.2.9+incompatible
	github.com/aliyun/aliyun-tablestore-go-sdk v4.1.2+incompatible
	github.com/apparentlymart/go-cidr v1.1.0
	github.com/apparentlymart/go-shquot v0.0.1
	github.com/apparentlymart/go-userdirs v0.0.0-20200915174352-b0c018a67c13
	github.com/apparentlymart/go-versions v1.0.3
	github.com/armon/circbuf v0.0.0-20190214190532-5111143e8da2
	github.com/aws/aws-sdk-go-v2 v1.36.3
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.30
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.43.2
	github.com/aws/aws-sdk-go-v2/service/kms v1.37.6
	github.com/aws/aws-sdk-go-v2/service/s3 v1.80.1
	github.com/aws/smithy-go v1.22.3
	github.com/bgentry/speakeasy v0.2.0
	github.com/bmatcuk/doublestar/v4 v4.9.1
	github.com/chzyer/readline v1.5.1
	github.com/cli/browser v1.3.0
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/dylanmei/winrmtest v0.0.0-20210303004826-fbc9ae56efb6
	github.com/go-logr/stdr v1.2.2
	github.com/go-test/deep v1.1.0
	github.com/go-viper/mapstructure/v2 v2.4.0
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/googleapis/gax-go/v2 v2.12.0
	github.com/hashicorp/aws-sdk-go-base/v2 v2.0.0-beta.65
	github.com/hashicorp/consul/api v1.13.0
	github.com/hashicorp/consul/sdk v0.8.0
	github.com/hashicorp/copywrite v0.16.3
	github.com/hashicorp/errwrap v1.1.0
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/go-getter v1.8.2
	github.com/hashicorp/go-hclog v1.6.3
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-plugin v1.7.0
	github.com/hashicorp/go-retryablehttp v0.7.8
	github.com/hashicorp/go-tfe v1.53.0
	github.com/hashicorp/go-uuid v1.0.3
	github.com/hashicorp/go-version v1.6.0
	github.com/hashicorp/hcl v1.0.1-vault-5
	github.com/hashicorp/hcl/v2 v2.20.1
	github.com/hashicorp/jsonapi v1.5.0
	github.com/jmespath/go-jmespath v0.4.0
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/lib/pq v1.10.3
	github.com/masterzen/winrm v0.0.0-20200615185753-c42b5136ff88
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-shellwords v1.0.12
	github.com/mitchellh/cli v1.1.5
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db
	github.com/mitchellh/copystructure v1.2.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/go-linereader v0.0.0-20190213213312-1b945b3263eb
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/mitchellh/gox v1.0.1
	github.com/mitchellh/reflectwalk v1.0.2
	github.com/nishanths/exhaustive v0.7.11
	github.com/openbao/openbao/api/v2 v2.3.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/opentofu/registry-address/v2 v2.0.0-20250611143131-d0a99bd8acdd
	github.com/opentofu/svchost v0.0.0-20250610175836-86c9e5e3d8c8
	github.com/packer-community/winrmcp v0.0.0-20180921211025-c76d91c1e7db
	github.com/pkg/errors v0.9.1
	github.com/posener/complete v1.2.3
	github.com/spf13/afero v1.9.3
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.0.588
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sts v1.0.587-0.20230206000712-97469a3dcd4e
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tag v1.0.233-0.20210823002710-8078545fa058
	github.com/tencentyun/cos-go-sdk-v5 v0.7.29
	github.com/xanzy/ssh-agent v0.3.3
	github.com/xlab/treeprint v1.2.0
	github.com/zclconf/go-cty v1.17.0
	github.com/zclconf/go-cty-debug v0.0.0-20240509010212-0d6042c53940
	github.com/zclconf/go-cty-yaml v1.1.0
	go.opentelemetry.io/contrib/exporters/autoexport v0.0.0-20230703072336-9a582bd098a2
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.46.1
	go.opentelemetry.io/otel v1.38.0
	go.opentelemetry.io/otel/sdk v1.38.0
	go.opentelemetry.io/otel/trace v1.38.0
	go.uber.org/mock v0.4.0
	golang.org/x/crypto v0.41.0
	golang.org/x/mod v0.26.0
	golang.org/x/net v0.43.0
	golang.org/x/oauth2 v0.32.0
	golang.org/x/sync v0.17.0
	golang.org/x/sys v0.37.0
	golang.org/x/term v0.36.0
	golang.org/x/text v0.28.0
	golang.org/x/tools v0.35.0
	google.golang.org/api v0.155.0
	google.golang.org/grpc v1.76.0
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.3.0
	google.golang.org/protobuf v1.36.10
	honnef.co/go/tools v0.4.2
	k8s.io/api v0.23.4
	k8s.io/apimachinery v0.23.4
	k8s.io/client-go v0.23.4
	k8s.io/utils v0.0.0-20211116205334-6203023598ed
	oras.land/oras-go/v2 v2.6.0
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.2.0 // indirect
	github.com/Azure/go-autorest/autorest v0.11.24 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.4.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	golang.org/x/exp v0.0.0-20230905200255-921286631fa9 // indirect
)

require (
	cloud.google.com/go v0.112.0 // indirect
	cloud.google.com/go/compute/metadata v0.7.0 // indirect
	cloud.google.com/go/iam v1.1.5 // indirect
	github.com/AlecAivazis/survey/v2 v2.3.6 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.10.1
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage v1.8.0
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys v1.4.0
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.6.1
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.18 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/Azure/go-ntlmssp v0.0.0-20200615164410-66371956d46c // indirect
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/ChrisTrenkamp/goxpath v0.0.0-20190607011252-c5096ec8773d // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/Masterminds/sprig/v3 v3.2.2 // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/antchfx/xmlquery v1.3.5 // indirect
	github.com/antchfx/xpath v1.1.10 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/armon/go-metrics v0.0.0-20180917152333-f0300d1749da // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20200907205600-7a23bdc65eef // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.10 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.29.15 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.68 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/service/iam v1.42.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.7.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.10.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/sns v1.34.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sqs v1.38.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.20 // indirect
	github.com/bgentry/go-netrc v0.0.0-20140422174119-9fd32a8b3d3d // indirect
	github.com/bradleyfalzon/ghinstallation/v2 v2.1.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cli/go-gh v1.0.0 // indirect
	github.com/cli/safeexec v1.0.0 // indirect
	github.com/cli/shurcooL-graphql v0.0.2 // indirect
	github.com/cloudflare/circl v1.6.1 // indirect
	github.com/creack/pty v1.1.18 // indirect
	github.com/dylanmei/iso8601 v0.1.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-jose/go-jose/v3 v3.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-openapi/errors v0.20.2 // indirect
	github.com/go-openapi/strfmt v0.21.3 // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/go-github/v45 v45.2.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.16.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.0.0 // indirect
	github.com/hashicorp/go-msgpack v0.5.4 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.2.0 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-slug v0.16.3 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/hashicorp/serf v0.9.6 // indirect
	github.com/hashicorp/terraform-plugin-log v0.9.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/henvic/httpretty v0.0.6 // indirect
	github.com/huandu/xstrings v1.3.3 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/jedib0t/go-pretty v4.3.0+incompatible // indirect
	github.com/jedib0t/go-pretty/v6 v6.4.4 // indirect
	github.com/joho/godotenv v1.3.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.15.11 // indirect
	github.com/knadh/koanf v1.5.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/masterzen/simplexml v0.0.0-20190410153822-31eea3082786 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mergestat/timediff v0.0.3 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/mitchellh/iochan v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mozillazg/go-httpheader v0.3.0 // indirect
	github.com/muesli/termenv v0.12.0 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/samber/lo v1.37.0 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/cobra v1.6.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/thanhpk/randstr v1.0.4 // indirect
	github.com/thlib/go-timezone-local v0.0.0-20210907160436-ef149e42d28e // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.mongodb.org/mongo-driver v1.11.6 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws v0.61.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.46.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.21.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.21.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.21.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/proto/otlp v1.0.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20221208152030-732eee02a75a // indirect
	golang.org/x/time v0.11.0 // indirect
	golang.org/x/tools/go/expect v0.1.1-deprecated // indirect
	google.golang.org/genproto v0.0.0-20240123012728-ef4313101c80 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.66.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.30.0 // indirect
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65 // indirect
	sigs.k8s.io/json v0.0.0-20211020170558-c049b76a60c6 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.1 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

go 1.25.2

replace github.com/hashicorp/hcl/v2 v2.20.1 => github.com/opentofu/hcl/v2 v2.20.2-0.20250121132637-504036cd70e7
