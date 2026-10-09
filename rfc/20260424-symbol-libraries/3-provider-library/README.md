This simplified example shows how symbol libraries could be used in conjunction with providers.

Prior to symbol libraries, you would need to have a custom type definition for each variable you wished to pass the resource into. Two approaches are possible, defining only the nessesary fields at the time or defining all fields in the object. The simpler approach is to only define the fields you need, though this starts to fall apart at scale as you pass the value into a chain of modules that requires additional fields. The more robust approach is to define all fields, with the downside of having to update ever instance of the copy-pasted type when performing a provider upgrade.

If end users or provider authors distribute symbol files alongside their providers, the above problems are moot. On a breaking provider upgrade, you simply bump the symbol library versions across your projects.

Provider authors now have the flexibility to include helper functions alongside their resources, for example converting an r2_storage resource into an object that can mimic an s3_bucket resource.


The "symbol library version upgrade to match the provider" process could be made easier by allowing symbols to be loaded from provider release archives or generating types automatically. https://github.com/opentofu/opentofu/issues/2704 discusses these ideas and their potential issues in more depth.
