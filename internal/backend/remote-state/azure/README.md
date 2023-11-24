# Azure State Backend

This README serves as a guide for developing the Azure State Backend.

## Running Integration Tests

The package contains multiple integration tests which need to be run with a live Azure account. This guide assumes you are using a fresh and empty Azure account/subscription. This way you'll be able to wipe it clean at the end without needing to worry about lingering resources.

You'll also need the azure CLI installed and configured with `az login`.

First, you'll need to configure the CLI to use the right subscription, in case your account has multiple subscriptions:

```bash
~> az account set --subscription <subscription_id>
```

You'll also need to create a service account, via
```bash
~> az ad sp create-for-rbac --role="Owner" --scopes="/subscriptions/<subscription_id>"
{
  "appId": "{APP_ID}",
  "displayName": "{DISPLAY_NAME}",
  "password": "{PASSWORD}",
  "tenant": "{TENANT}"
}
```
We'll also need a certificate for the service account, as there are tests which check certificate authentication.
```bash
# Generating key+cert pair.
~> openssl req -subj '/CN=myclientcertificate/O=MyCompany, Inc./ST=CA/C=US' \
 -new -newkey rsa:4096 -sha256 -days 3 -nodes -x509 -keyout client.key -out client.crt
# Creating a pfx bundle with the format required by the state backend.
~> openssl pkcs12 -certpbe PBE-SHA1-3DES -keypbe PBE-SHA1-3DES -export -macalg sha1 -password "pass:" -out client.pfx -inkey client.key -in client.crt
```

You will now have to **use the UI** to add this certificate. Go to `App Registrations` in the Azure Portal, pick the app with the previously generated `{DISPLAY_NAME}`, there you go into `Certificates & secrets`, Certificates tab, and `Upload certificate` with the `client.crt` file.

You'll now want to compile the tests. We'll be running them later on a VM (so that tests checking IMDS authentication work). Go to the `internal/backend/remote-state/azure` directory and run:
```bash
~> GOOS=linux GOARCH=amd64 go test -c .
```

Create a resource group for your Azure VM:
```bash
~> az group create --name myResourceGroup --location eastus
```

Now, let's create the Azure VM.
```bash
~> az vm create --resource-group myResourceGroup --name myVM --image Ubuntu2204 --generate-ssh-keys --admin-username azureuser --admin-password <long password with lower and upper letters, numbers and symbols>
{
  "fqdns": "",
  "id": "...",
  "location": "eastus",
  "macAddress": "...",
  "powerState": "VM running",
  "privateIpAddress": "...",
  "publicIpAddress": "{PUBLIC_IP_ADDRESS}",
  "resourceGroup": "myResourceGroup",
  "zones": ""
}
```
Assign an identity to the VM:
```bash
~> az vm identity assign --resource-group myResourceGroup --name myVM
{
  "systemAssignedIdentity": "{IDENTITY}",
  "userAssignedIdentities": {}
}
```

and a role to that identity:
```bash
~> az role assignment create --assignee "{IDENTITY}" --role Owner --scope "/subscriptions/<subscription_id>"
```

You'll now want to copy the compiled tests and certificate to the vm:
```bash
# This might hang for a bit, while the VM is booting up.
~> scp azure.test client.pfx azureuser@{PUBLIC_IP_ADDRESS}:~/
~> ssh azureuser@{PUBLIC_IP_ADDRESS}
```

Now, on the Azure VM bash session we'll have to set up the environment variables for the tests:
```bash
export TF_AZURE_TEST=1
export TF_RUNNING_IN_AZURE=1
export ARM_SUBSCRIPTION_ID=<subscription_id>
export ARM_LOCATION=eastus
export ARM_ENVIRONMENT=public
export ARM_TENANT_ID={TENANT}
export ARM_CLIENT_ID={APP_ID}
export ARM_CLIENT_SECRET={PASSWORD}
export ARM_CLIENT_CERTIFICATE_PATH=/home/azureuser/client.pfx
```

Finally, we can run the tests!
```bash
~> ./azure.test -test.v -test.timeout 99999s
```
The tests should run for around 30 minutes. Enjoy your coffee!

### Cleanup

Now it's time to get rid of everything we've created.

List all resource groups in your subscription:
```bash
~> az group list --subscription <subscription_id> --query "[].name"
[
  "myResourceGroup",
  "acctestRG-backend-23112414590786-k3nx",
  "..."
]
```

For each of these, run:
```bash
~> az group delete --subscription <subscription_id> --name <resource_group_name> --yes --no-wait --force-deletion-types "Microsoft.Compute/virtualMachines"
```

You'll also want to delete the service account:
```bash
~> az ad sp delete --id {APP_ID}
```

List ServicePrincipal role assignments in the subscription:
```bash
~> az role assignment list --subscription <subscription_id> --query "[?principalType=='ServicePrincipal']"
[
  {
    "canDelegate": null,
    "condition": null,
    "conditionVersion": null,
    "description": null,
    "id": "{ASSIGNMENT_ID}",
    "name": "...",
    "principalId": "...",
    "principalType": "ServicePrincipal",
    "resourceGroup": "",
    "roleDefinitionId": "/subscriptions/<subscription_id>/providers/Microsoft.Authorization/roleDefinitions/...",
    "scope": "/subscriptions/<subscription_id>",
    "type": "Microsoft.Authorization/roleAssignments"
  },
  ...
]
```

and for each of those, delete it:
```bash
~> az role assignment delete --subscription <subscription_id> --id {ASSIGNMENT_ID}
```

At this point, double-check that all resource groups are gone:
```bash
~> az group list --subscription <subscription_id> --query "[].name"
[]
```
