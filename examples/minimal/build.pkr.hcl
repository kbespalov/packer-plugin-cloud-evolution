packer {
  required_plugins {
    cloud-evolution = {
      version = ">= 0.1.0"
      source  = "github.com/kbespalov/cloud-evolution"
    }
  }
}

# Packer allows env() only as a variable default, not inside source {}.
# The builder also reads CLOUDRU_EVOLUTION_* when a field is left empty.

variable "key_id" {
  type    = string
  default = env("CLOUDRU_EVOLUTION_KEY_ID")
}

variable "key_secret" {
  type      = string
  default   = env("CLOUDRU_EVOLUTION_KEY_SECRET")
  sensitive = true
}

variable "project_id" {
  type    = string
  default = env("CLOUDRU_EVOLUTION_PROJECT_ID")
}

variable "zone_id" {
  type        = string
  default     = env("CLOUDRU_EVOLUTION_ZONE_ID")
  description = "Availability zone UUID, not the display name (ru.AZ-2)."
}

variable "subnet_id" {
  type        = string
  default     = env("CLOUDRU_EVOLUTION_SUBNET_ID")
  description = "VPC subnet UUID for the builder VM."
}

variable "flavor_id" {
  type        = string
  default     = env("CLOUDRU_EVOLUTION_FLAVOR_ID")
  description = "Compute flavor UUID."
}

variable "source_image_id" {
  type        = string
  default     = env("CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID")
  description = "Source catalog image UUID (public Ubuntu-24.04 is fine)."
}

variable "image_name" {
  type        = string
  default     = "example-golden"
  description = "Private image name; unique in the project. Override with -var or CLOUDRU_EVOLUTION_IMAGE_NAME."
}

source "cloud-evolution" "minimal" {
  key_id          = var.key_id
  key_secret      = var.key_secret
  project_id      = var.project_id
  zone            = var.zone_id
  subnet_id       = var.subnet_id
  flavor_id       = var.flavor_id
  source_image_id = var.source_image_id
  image_name      = var.image_name
  ssh_username    = "ubuntu"
  ssh_timeout     = "10m"
  state_timeout   = "40m"

  temporary_key_pair_type = "ed25519"
  pause_before_connecting = "20s"
}

build {
  sources = ["source.cloud-evolution.minimal"]

  provisioner "shell" {
    inline = [
      "sudo cloud-init status --wait || true",
      "install -d -m 0755 /tmp/cloud-evolution && date -u > /tmp/cloud-evolution/baked-at",
    ]
  }
}
