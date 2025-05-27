
# This intentionally refers to the registry of OpenTofu's predecessor, but
# the associated configuration refers to the shorthand "hashicorp/null"
# and so will be understood by OpenTofu as depending instead on
# "registry.opentofu.org/hashicorp/null", thereby activating our special
# fixup behavior and selecting the same version of OpenTofu's re-release
# of this provider.
provider "registry.terraform.io/hashicorp/null" {
  version = "3.2.0"
  hashes = [
    "h1:uQv2oPjJv+ue8bPrVp+So2hHd90UTssnCNajTW554Cw=",
    "zh:40335019c11e5bdb3780301da5197cbc756b4b5ac3d699c52583f1d34e963176",
    "zh:42356e687656fc8ec1f230f786f830f344e64419552ec483e2bc79bd4b7cf1e8",
    "zh:5ce03460813954cbebc9f9ad5befbe364d9dc67acb08869f67c1aa634fbf6d6c",
    "zh:658ea3e3e7ecc964bdbd08ecde63f3d79f298bab9922b29a6526ba941a4d403a",
    "zh:68c06703bc57f9c882bfedda6f3047775f0d367093d00efb040800c798b8a613",
    "zh:80fd03335f793dc54302dd53da98c91fd94f182bcacf13457bed1a99ecffbc1a",
    "zh:91a76f371815a130735c8fcb6196804d878aebcc67b4c3b73571d2063336ffd8",
    "zh:c146fc0291b7f6284fe4d54ce6aaece6957e9acc93fc572dd505dfd8bcad3e6c",
    "zh:c38c9a295cfae9fb6372523c34b9466cd439d5e2c909b56a788960d387c24320",
    "zh:e25265d4e87821d18dc9653a0ce01978a1ae5d363bc01dd273454db1aa0309c7",
  ]
}
