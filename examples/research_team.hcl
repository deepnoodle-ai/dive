
name = "Research Team"

description = "A team of agents that will research a topic"

config {
  log_level = "debug"
  default_provider = "anthropic"
}

// Variables

variable "team_name" {
  type = "string"
  description = "The name of the research team"
  default {
    value = "Elite Research Team"
  }
}

variable "supervisor_name" {
  type = "string"
  description = "The name of the supervisor agent"
  default {
    value = "Joe"
  }
}

variable "assistant_name" {
  type = "string"
  description = "The name of the research assistant agent"
  default {
    value = "Holly"
  }
}

variable "research_topic" {
  type = "string"
  description = "The topic to research"
  default {
    value = "Maple syrup production in Vermont"
  }
}

// Tools

tool "google_search" {
  enabled = true
}

tool "firecrawl" {
  enabled = true
}

// Agents

agent "Supervisor" {
  name = var.supervisor_name
  description = format("Research Supervisor and Renowned Author. Assign research tasks to %s, but prepare the final reports or biographies yourself.", var.assistant_name)
  is_supervisor = true
  subordinates = [var.assistant_name]
}

agent "Research Assistant" {
  name = var.assistant_name
  description = "You are an expert research assistant. Don't go too deep into the details unless specifically asked."
  tools = ["google_search", "firecrawl"]
}

// Tasks

task "Background Research" {
  description = format("Gather background research that will be used to create a history of %s. Don't consult more than one source. The goal is to produce about 3 paragraphs of research - that is all. Don't overdo it.", var.research_topic)
  assigned_agent = var.assistant_name
}

task "Write History" {
  description = format("Create a brief 3 paragraph history of %s.", var.research_topic)
  expected_output = "The history, with the first word of each paragraph in ALL UPPERCASE"
  assigned_agent = var.supervisor_name
  dependencies = ["Background Research"]
  output_file = format("%s_history.txt", replace(var.research_topic, " ", "_"))
}
