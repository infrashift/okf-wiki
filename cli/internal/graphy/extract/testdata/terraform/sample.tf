variable "region" {
  type    = string
  default = "us-east-1"
}

variable "instance_type" {
  type = string
}

locals {
  name_prefix = "web-${var.region}"
}

provider "aws" {
  region = var.region
}

data "aws_ami" "ubuntu" {
  most_recent = true
}

resource "aws_instance" "web" {
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.instance_type

  tags = {
    Name = local.name_prefix
  }
}

module "vpc" {
  source     = "./modules/vpc"
  cidr_block = "10.0.0.0/16"
}

output "instance_ip" {
  value = aws_instance.web.public_ip
}
