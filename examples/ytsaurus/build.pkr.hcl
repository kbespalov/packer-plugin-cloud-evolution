packer {
  required_plugins {
    cloud-evolution = {
      version = ">= 0.1.0"
      source  = "github.com/kbespalov/cloud-evolution"
    }
  }
}

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
  default     = "ytsaurus-ubuntu-24-04"
  description = "Private image name; unique in the project. Override with -var or CLOUDRU_EVOLUTION_IMAGE_NAME."
}

source "cloud-evolution" "ytsaurus" {
  key_id            = var.key_id
  key_secret        = var.key_secret
  project_id        = var.project_id
  zone              = var.zone_id
  subnet_id         = var.subnet_id
  flavor_id         = var.flavor_id
  source_image_id   = var.source_image_id
  image_name        = var.image_name
  image_description = "YTsaurus / tractoai node golden (Ubuntu, Docker, sysctl)"
  disk_size_gb      = 20
  linux_login       = "ubuntu"
  ssh_username      = "ubuntu"
  ssh_timeout       = "10m"
  state_timeout     = "40m"

  temporary_key_pair_type = "ed25519"
  pause_before_connecting = "20s"
}

build {
  sources = ["source.cloud-evolution.ytsaurus"]

  provisioner "shell" {
    script          = "${path.root}/scripts/provision.sh"
    execute_command = "sudo -n -E bash -c '{{ .Vars }} {{ .Path }}'"
  }
}
