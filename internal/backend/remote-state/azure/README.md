Refer to the [official documentation](https://opentofu.org/docs/language/settings/backends/azurerm/) for a quick reference of available authentication methods.

# Running Tests

The files `backend_test.go` and `client_test.go` contain various unit tests and acceptance tests for ensuring the backend state management is running properly. Acceptance tests are any test whose name begins with `TestAcc...`; everything else is a unit test.

You should be able to run unit tests without any further configuration, and acceptance tests are skipped by default.

Note: All tests assume you are running on Azure Public Cloud. These tests were not made to work in special environments (like Azure China, Azure Government, or Azure Stack) and will fail if you try to run them there.

## Running Acceptance Tests

You will need to set the following environment variables in order to run the acceptance tests:

```bash
export TF_AZURE_TEST=1
export TF_ACC=1
```

Additionally, you'll need to set your Azure location, subscription id, and tenant id;

```bash
export ARM_LOCATION=centralus
export ARM_SUBSCRIPTION_ID='xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'
export ARM_TENANT_ID='xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'
```

We recommend using the Azure CLI (`az`) to authenticate to Azure for the infrastructure bootstrapping steps, which create a resource group, storage account, and blob storage container in your configured subscription.
It's also possible to run these tests by setting TF_AZURE_TEST_CLIENT_ID and TF_AZURE_TEST_CLIENT_SECRET if the Azure CLI cannot be used.

With all of these configured, you'll be able to run the following tests:
- TestAccBackendAccessKeyBasic
- TestAccBackendSASToken
- TestAccRemoteClientAccessKeyBasic
- TestAccRemoteClientSASToken

Besides these tests, every other acceptance test requires authentication by a service principal or client. The utility workspace in the `meta-test` folder will create the application for you, and will also manage the credentials. If you would like to do this manually, [follow this guide](https://learn.microsoft.com/en-us/entra/identity-platform/quickstart-register-app) for creating an application registration. You'll note on the side that there is a section called "Certificates & secrets". You will want to use this for the client secret and client certificate tests below.

### Running Client Secret tests

To run the secrets test, you will need these variables:

```bash
export TF_AZURE_TEST_CLIENT_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export TF_AZURE_TEST_CLIENT_SECRET=some~secret~string
```

You can get those by going into the meta-test directory and following the instructions there, or manually obtaining a secret client. Instructions for manually creating a Client Secret for your existing app registration can be found [here](https://learn.microsoft.com/en-us/azure/industry/training-services/microsoft-community-training/public-preview-version/frequently-asked-questions/generate-new-clientsecret-link-to-key-vault#check-and-update-client-secret-expiration-date) in the first part of this guide. You do not need to put the secret in a vault or update the application configuration.

With these additional environment variables configured, you'll be able to run the following tests:
- TestAccBackendServicePrincipalClientSecret
- TestAccRemoteClientServicePrincipalClientSecret

### Running Client Certificate test

To run the certificates test, you will need these variables:

```bash
export TF_AZURE_TEST_CLIENT_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export TF_AZURE_TEST_CERT_PATH="meta-test/certs.pfx"
export TF_AZURE_TEST_CERT_PASSWORD=sOmEpAsSwOrD
```

If you apply the meta-test workspace, the certificate will be generated for you and attached to an appropriately-permissioned application. Otherwise, you can generate your own certificate with the `openssl` utility command:

```bash
# Generating key+cert pair.
~> openssl req -subj '/CN=myclientcertificate/O=MyCompany, Inc./ST=CA/C=US' \
 -new -newkey rsa:4096 -sha256 -days 3 -nodes -x509 -keyout client.key -out client.crt
# Creating a pfx bundle with the format required by the state backend.
~> openssl pkcs12 -certpbe PBE-SHA1-3DES -keypbe PBE-SHA1-3DES -export -macalg sha1 -password "pass:" -out client.pfx -inkey client.key -in client.crt
```

You will then go to the Azure Portal UI and manually upload the public `client.crt` file to the certificates for your application.

### Running Managed Service Identity test

We strongly recommend using the workspace in the `meta-test` folder to set up the VM and associated authorizations.

Within the same directory as this README, compile all the tests:

```bash
$ GOOS=linux GOARCH=amd64 go test -c .
```

This will generate an `azure.test` file. Send this to your VM:

```bash
$ scp azure.test azureadmin@xxx.xxx.xxx.xxx:
```

Now, SSH into your VM:

```bash
$ ssh azureadmin@xxx.xxx.xxx.xxx
```

Set up the following environment variables:

```bash
export TF_AZURE_TEST=1
export TF_ACC=1
export ARM_LOCATION=centralus
export ARM_SUBSCRIPTION_ID='xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'
export ARM_TENANT_ID='xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'
export TF_AZURE_TEST_STORAGE_ACCOUNT_NAME=acctestsaxxxx
export TF_AZURE_TEST_RESOURCE_GROUP_NAME=acctestRG-backend-1234567890-xxxx
export TF_AZURE_TEST_CONTAINER_NAME=acctestcont
```

Finally, run the MSI test:

```bash
$ ./azure.test -test.v -test.run "TestAcc.*ManagedServiceIdentity"
```

### Running AKS Workload Identity Test

We strongly recommend using the workspace in the `meta-test` folder to set up the AKS Kubernetes cluster and associated authorizations.

Within the same directory as this README, compile all the tests:

```bash
$ GOOS=linux GOARCH=amd64 go test -c .
```

This will generate an `azure.test` file. Assuming that `kubectl` is configured to go to a pod named `shell-demo` in the `default` namespace, run the following command:

```bash
kubectl cp azure.test shell-demo:/
```

Shell into the pod:

```bash
kubectl exec --stdin --tty shell-demo -- /bin/sh
```

Set up the following environment variables:

```bash
export TF_AZURE_TEST=1
export TF_ACC=1
export ARM_LOCATION=centralus
export ARM_SUBSCRIPTION_ID='xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'
export ARM_TENANT_ID='xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'
export TF_AZURE_TEST_STORAGE_ACCOUNT_NAME=acctestsaxxxx
export TF_AZURE_TEST_RESOURCE_GROUP_NAME=acctestRG-backend-1234567890-xxxx
export TF_AZURE_TEST_CONTAINER_NAME=acctestcont
```

Finally, run the AKS Workload Identity test:

```bash
$ ./azure.test -test.v -test.run "TestAcc.*AKSWorkloadIdentity"
```
