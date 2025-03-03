
name = "Research Team"

description = "A expert research team of agents that will research any topic"

config {
  log_level = "debug"
  default_provider = "anthropic"
  output_dir = "output"
}

variable "topic" {
  type = "string"
  description = "The topic to research"
}

tool "google_search" {
  enabled = true
}

tool "firecrawl_scrape" {
  enabled = true
}

agent "supervisor" {
  description = "Expert research supervisor. Assign research tasks to the assistant. Prepare the final reports yourself."
  is_supervisor = true
  subordinates = [agents.assistant]
}

agent "assistant" {
  description = "You are an expert research assistant. When researching, don't go too deep into the details unless specifically asked."
  tools = [
    tools.google_search,
    tools.firecrawl_scrape,
  ]
}

task "research" {
  description = format("Gather background research on %s. Don't consult more than one source. The goal is to produce about 3 paragraphs of research - that is all. Don't overdo it.", var.topic)
  output_file = "research.txt"
}

task "report" {
  description = format("Create a brief 3 paragraph report on %s", var.topic)
  expected_output = "The history, with the first word of each paragraph in ALL UPPERCASE"
  assigned_agent = agents.supervisor
  dependencies = [tasks.research]
  output_file = "report.txt"
}
