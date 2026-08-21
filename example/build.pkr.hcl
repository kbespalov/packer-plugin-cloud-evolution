packer {
  required_plugins {
    cloud-evolution = {
      version = ">= 0.1.0"
      source  = "github.com/kbespalov/cloud-evolution"
    }
  }
}

variable "zone" {
  type = string
}

variable "subnet_id" {
  type = string
}

variable "flavor_id" {
  type = string
}

variable "source_image_id" {
  type = string
}

source "cloud-evolution" "golden" {
  key_id          = env("EVOLUTION_KEY_ID")
  key_secret      = env("EVOLUTION_KEY_SECRET")
  project_id      = env("EVOLUTION_PROJECT_ID")
  zone            = var.zone
  subnet_id       = var.subnet_id
  flavor_id       = var.flavor_id
  source_image_id = var.source_image_id
  image_name      = "example-golden"
  ssh_username    = "ubuntu"
}

build {
  sources = ["source.cloud-evolution.golden"]

  provisioner "shell" {
    inline = [
      "sudo cloud-init status --wait || true",
    ]
  }
}
