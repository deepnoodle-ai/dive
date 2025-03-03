
name = "File Explorer"

description = "List files in a directory"

config {
  log_level = var.log_level
  default_provider = var.provider
  default_model = var.model
}

variable "provider" {
  type = string
  description = "The default provider to use for the team"
  default = "anthropic"
}

variable "model" {
  type = string
  description = "The default model to use for the team"
  default = "claude-3-7-sonnet-20250219"
}

variable "log_level" {
  type = string
  description = "The log level to use for the team"
  default = "info"
}

variable "directory" {
  type = string
  description = "The directory to list files in"
  default = "."
}

tool "google_search" {
  enabled = true
}

tool "firecrawl_scrape" {
  enabled = true
}

tool "directory_list" {
  enabled = true
}

tool "file_read" {
  enabled = true
}

agent "engineer" {
  description = "Principal Software Engineer"
  tools = [
    tools.google_search,
    tools.firecrawl_scrape,
    tools.directory_list,
    tools.file_read,
  ]
}

task "list-files" {
  description = format("List all files in the directory: %s", var.directory)
  assigned_agent = agents.engineer
}
