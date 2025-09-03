This folder provides the necessary infrastructure for the acceptance tests in this backend. In order to use this, you must have administrative privileges within your Azure Subscription.

We recommend using CLI authentication and setting the subscription using the ARM_SUBSCRIPTION_ID environment variable:

```bash
$ az login
$ export ARM_SUBSCRIPTION_ID="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
```

Once those are set, you can initialize and apply the OpenTofu workspace:

```bash
$ tofu init
$ tofu apply
# When you're ready to obtain the secrets through environment variables
$ tofu apply -show-sensitive
```

You should see some environment variables that look like this:

```bash
export TF_AZURE_TEST_CLIENT_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export TF_AZURE_TEST_SECRET=some~secret~string
```

Copy and paste these into the command line to provide the secrets for the backend tests.

A file called `certs.pfx` should also be created, which can be placed in an appropriate directory and used for certificate authentication by setting the path appropriately, perhaps something like:

```bash
export TF_AZURE_TEST_CERT_PATH="meta-test/certs.pfx"
export TF_AZURE_TEST_CERT_PASSWORD=SoMePaSsWoRd
```

### Managed Service Identity

By default, the virtual machine, identity, and associated authorizations required for MSI testing are not set up by this workspace. In order to set those up, you need some extra variables:

```bash
$ tofu apply -show-sensitive -var 'use_msi=true' -var 'location=centralus' -var 'ssh_pub_key_path=~/.ssh/id_rsa.pub'
```

The last two variables are optional; the defaults have been provided here. After running this, some additional environment variables will be available.

Additionally, there should be a small output with the ssh instructions, and the IP address of the VM:

```bash
ssh_instructions = "ssh azureadmin@xxx.xxx.xxx.xxx"
```

In order to tear down msi infrastructure, while keeping the rest of the credentials and setup, simply run the `tofu apply` without the `use_msi=true` variable.

### Cleanup

Simply run

```bash
$ tofu destroy
```

... and all the test infrastructure will be deleted.
