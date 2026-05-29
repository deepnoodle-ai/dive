# NoodleVille

NoodleVille is a tiny generative-agent town demo for Dive. It runs a configurable
town in the terminal: one goroutine per villager, a bounded LLM worker pool,
typed action tools, per-villager file sessions, periodic reflection/compaction,
and a small persistent memory stream.

Run the default path:

```sh
go run .
```

By default NoodleVille uses `-provider auto`: it runs on a local Ollama model
when one is available, and falls back to the deterministic scripted planner when
Ollama is not reachable.

Run the deterministic smoke path explicitly:

```sh
go run . -provider scripted -ticks 4 -villagers 12 -reflect-every 2
```

Run with local Ollama:

```sh
ollama pull llama3.2:3b
go run . -provider ollama -model llama3.2:3b -ticks 6 -villagers 12 -parallelism 2
```

Run the embedded browser view:

```sh
go run . -provider scripted -http :8080 -ticks 0
```

The browser view shows the live tile map, event feed, party propagation count,
and a villager inspector. Click a villager on the map to inspect their current
plan, social ties, party knowledge source, and recent memories.

Record a replayable run trace:

```sh
go run . -provider scripted -ticks 8 -record /tmp/noodleville-run.jsonl
```

The recording is newline-delimited JSON: a run header followed by one complete
tick report per line. It is meant to feed future timelapse and clip tooling
without adding a heavy video dependency to the demo.

Sessions are stored under `.noodleville/sessions` by default, so villagers keep
their turn history and memory entries across restarts.

The default run seeds Maya with a Saturday noodle party goal. Over several ticks
the idea moves through ordinary plans and conversations rather than a scripted
global event.
