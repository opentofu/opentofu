I'll give the different conventions in [the Style Conventions page](https://opentofu.org/docs/language/syntax/style/) numbers so they're easier to refer to. 

1. Indent two spaces for each nesting level.
1. When multiple arguments with single-line values appear on consecutive lines at the same nesting level, align their equals signs:
1. When both arguments and blocks appear together inside a block body, place all of the arguments together at the top and then place nested blocks below them. Use one blank line to separate the arguments from the blocks.
1. Use empty lines to separate logical groups of arguments within a block.
1. For blocks that contain both arguments and "meta-arguments" (as defined by the OpenTofu language semantics), list meta-arguments first and separate them from other arguments with one blank line. Place meta-argument blocks last and separate them from other blocks with one blank line.
1. Top-level blocks should always be separated from one another by one blank line. Nested blocks should also be separated by blank lines, except when grouping together related blocks of the same type (like multiple `provisioner` blocks in a resource).
1. Avoid grouping multiple blocks of the same type with other blocks of a different type, unless the block types are defined by semantics to form a family. (For example: `root_block_device`, `ebs_block_device` and `ephemeral_block_device` on `aws_instance` form a family of block types describing AWS block devices, and can therefore be grouped together and mixed.)

Convention 1 and 2 is already enforced by `tofu fmt`, so we don't need to discuss those.

---

## 3. Put arguments before blocks

> When both arguments and blocks appear together inside a block body, place all of the arguments together at the top and then place nested blocks below them. Use one blank line to separate the arguments from the blocks.

If I have a block like this:

```hcl
resource "aws_instance" "example" {
  ami = "abc123"
  network_interface {
    # ...
  }
  instance_type = "t2.micro"
}
```

I'd expect `tofu fmt` to move `instance_type` above the `network_interface` block so it ends up like this:

```hcl
resource "aws_instance" "example" {
  ami           = "abc123"
  instance_type = "t2.micro" # moved above the network_interface block

  network_interface {
    # ...
  }
}
```

### Discussion / comment / questions

- How do we handle comments or comment blocks above arguments and blocks? For example:
  ```hcl
  resource "aws_instance" "example" {
    ami = "abc123"
    # this comment is written over multiple
    # lines and could have been a /* block comment */
    network_interface {
      # ...
    }
    # Use t2.micro because it's part of the AWS free tier
    instance_type = "t2.micro"
  }
  ```
   Should we assume the comment(s) above an argument or block is related to it, should we ignore it, or should the behavior be configurable by the user?
  - I think we should assume it's related. My impression is having comments related to an argument or block _below_ the argument or block is uncommon, I'm sure it happens but I can't remember seeing it.
- Should we treat multi-line lists and maps the same as single-line arguments?
  - As stated in convention 2 (already enforced by `tofu fmt`), multi-line lists and maps won't have their equal signs aligned, for example:
    ```hcl
    vpc_config {
      subnet_ids = [ # not aligned with the other arguments
        aws_subnet.subnet_for_lambda.id
      ]
      security_group_ids          = [aws_security_group.sg_for_lambda.id]
      ipv6_allowed_for_dual_stack = false
    }
    ```
    So singe-line and multi-line lists and maps are already formatted differently.
  - I like the idea to place them last in the list of arguments or groups of arguments (see convention 4.) So this example: (Note: this is not a convention mentioned in the current Style Conventions doc)
    ```hcl
    module "foo" {
      source = "modules/example"
      
      name = "foo"
      
      vpc_id = "vpc_id"
      security_group_ids = [
        local.sg_1,
        local.sg_2,
      ]
      subnets = ["subset-1", "subnet-2"]
      
      role = local.role_arn
    }
    ```
    Would be formatted to:
    ```hcl
    module "foo" {
      source = "modules/example"
      
      name = "foo"
      
      vpc_id  = "vpc_id" # now has the equal sign aligned with subnets
      subnets = ["subset-1", "subnet-2"]
      security_group_ids = [
        local.sg_1,
        local.sg_2,
      ] # multi-line list placed last in the argument group
      
      role = local.role_arn
    }
    ```

## 4. Use empty lines to separate logical groups of arguments within a block.

I'm not certain what this convention means, but I imagine something like this:
```hcl
resource "aws_lambda_function" "example" {
  function_name = "lambda_function_name"

  filename         = "lambda_function_payload.zip"
  source_code_hash = data.archive_file.lambda.output_base64sha256
  
  handler          = "index.test"
  runtime          = "nodejs18.x"
  
  role = aws_iam_role.iam_for_lambda.arn
}
```
The `filename` is related to `source_code_hash` because their arguments likely relate to the same source. `handler` and `runtime` can be considered related since `handler` is what `runtime` should execute.

### Discussion / comment / questions

- Judging what belongs to the same "logical group" seems subjective and is therefore probably hard to automate.
  - I suggest `tofu fmt` does not try to group blocks automatically, but rather _allows_ groups separated with empty lines to exist (i.e. don't automatically remove empty lines between arguments)
  
## 5. Place meta-arguments first and meta-blocks last, and separate with a blank line
  
> For blocks that contain both arguments and "meta-arguments" (as defined by the OpenTofu language semantics), list meta-arguments first and separate them from other arguments with one blank line. Place meta-argument blocks last and separate them from other blocks with one blank line.
  
The example from the Style Convention page:
  
```hcl
resource "aws_instance" "example" {
  count = 2 # meta-argument first

  ami           = "abc123"
  instance_type = "t2.micro"

  network_interface {
    # ...
  }

  lifecycle { # meta-argument block last
    create_before_destroy = true
  }
}
```

### Discussion / comment / questions

- As with 3, we need to figure out how to handle comments
- Should we also place `depends_on` an the top? Common convention seem to be to place it last/treat it like a block.
  - The example in the [depends_on](https://opentofu.org/docs/language/meta-arguments/depends_on/) documentation it's placed last for example.
  - Personally I prefer it last, but if we want to avoid exceptions I'm okay with having it first just like the other arguments.
- There are only [6 meta-arguments and blocks defined](https://opentofu.org/docs/language/meta-arguments/count/), so they could be treated on a case-by-case basis unless we expect the number to grow a lot.
  - We'd need to remember or enforce making a decision for this when adding new meta-arguments, which could be difficult.

## 6. Blocks should be separated by one blank line

> Top-level blocks should always be separated from one another by one blank line. Nested blocks should also be separated by blank lines, except when grouping together related blocks of the same type (like multiple `provisioner` blocks in a resource).

I read this convention to present itself something like this:

```hcl
module "example" {
  source    = "./example"
}
# blank line here between top-level blocks
resource "aws_instance" "example" {
  count = 2

  ami           = "abc123"
  instance_type = "t2.micro"
  
  network_interface {
    # ...
  }
  # blank line here between nested blocks
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_instance" "foo" {
  # ...

  provisioner "local-exec" {
    command = "echo first"
  }
  provisioner "local-exec" { # no blank line between related blocks of the same type
    command = "echo second"
  }
}
```

### Discussion / comment / questions

- I think adding blank lines between blocks is in my opinion really nice.
- The convention says to _not_ have blank lines between similar blocks. I'm not sure I see the great benefit of this, and [official example for multiple privisioners](https://opentofu.org/docs/language/resources/provisioners/syntax/#multiple-provisioners) blank lines are used to separate them.
  - A compromise could be to _allow_ no blank lines between similar blocks, but not enforce it.
  - Another option could be to have this configurable.
- The rule says to group together _related_ blocks, so we'd need to define what's considered related. Like with convention 4, I judging what's related is subjective and will be hard to automate. I suggest not grouping resources automatically. If people want to group them/separate them from other blocks they may do so with a comment for example.
- If there is a comment above a block, should we add a blank line above or below the comment, or count the comment as a blank line?
  - I think we should add a blank line above the comment. So
    ```hcl
    resource "aws_instance" "example" {
      # ...
    }
    # our public website
    resource "aws_instance" "web" {
      # ...
    }
    ```
    Gets formatted to:
    ```hcl
    resource "aws_instance" "example" {
      # ...
    }
    
    # our public website
    resource "aws_instance" "web" {
      # ...
    }
    ```

## 7. Avoid grouping multiple blocks of the same type with other blocks of a different type, unless the block types are defined by semantics to form a family.

> Avoid grouping multiple blocks of the same type with other blocks of a different type, unless the block types are defined by semantics to form a family. (For example: `root_block_device`, `ebs_block_device` and `ephemeral_block_device` on `aws_instance` form a family of block types describing AWS block devices, and can therefore be grouped together and mixed.)

This convention says they _can_ be grouped, rather than _should_ or _must_ be grouped. I read this convention as an add-on to the 6th convention rather than a separate one.

### Discussion / comment / questions

- Automating what blocks are in a "family" sounds like it could be a bit of work, so I suggest we skip these exceptions at least initially.
- Other than the exceptions, I read this convention the same as 6 so if we implement 6 I think that's win already.

---

Other things to consider:

-  Do we want any or all of the conventions to be optional/configurable?
  - I suggest we add as few options as possible, but having an option to out in our out of the new behavior could probably be good (e.g. `tofu fmt --legacy` or `tofu fmt --strict`) to allow backwards compatibility.
- We also have the option of releasing one or two conventions at a time/iteratively rather than a big bang release with all conventions right off the bat.