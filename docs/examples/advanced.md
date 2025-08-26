# Advanced Examples

Complex real-world examples demonstrating advanced Dive patterns and integrations.

## 📋 Table of Contents

- [Multi-Agent Code Review System](#multi-agent-code-review-system)
- [Autonomous Research Pipeline](#autonomous-research-pipeline)
- [Customer Support Bot with Escalation](#customer-support-bot-with-escalation)
- [Content Generation Pipeline](#content-generation-pipeline)
- [Data Analysis and Reporting System](#data-analysis-and-reporting-system)
- [Automated Testing Coordinator](#automated-testing-coordinator)
- [Multi-Modal Document Processor](#multi-modal-document-processor)
- [Real-Time Trading Assistant](#real-time-trading-assistant)

## Multi-Agent Code Review System

A comprehensive code review system with multiple specialized agents working together.

### Configuration

```yaml
Name: Advanced Code Review System
Description: Multi-agent system for comprehensive code analysis

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info
  MaxConcurrency: 5

Agents:
  - Name: Security Reviewer
    Instructions: |
      You are a security expert who reviews code for vulnerabilities, 
      security best practices, and potential attack vectors.
      Focus on: SQL injection, XSS, authentication flaws, data exposure.
    Tools:
      - read_file
      - static_analysis
      - security_scan
    Temperature: 0.2
    
  - Name: Performance Reviewer
    Instructions: |
      You analyze code for performance issues, optimization opportunities,
      and scalability concerns. Focus on algorithms, database queries,
      memory usage, and computational complexity.
    Tools:
      - read_file
      - profiling_analysis
      - benchmark_runner
    Temperature: 0.3
    
  - Name: Quality Reviewer
    Instructions: |
      You review code quality, maintainability, and adherence to best practices.
      Focus on: code structure, naming, documentation, testing, SOLID principles.
    Tools:
      - read_file
      - lint_runner
      - test_coverage
    Temperature: 0.4
    
  - Name: Architecture Reviewer
    Instructions: |
      You evaluate architectural decisions, design patterns, and overall
      system design. Focus on: separation of concerns, dependency management,
      modularity, and long-term maintainability.
    Tools:
      - read_file
      - dependency_analyzer
      - design_pattern_detector
    Temperature: 0.3
    
  - Name: Review Coordinator
    Instructions: |
      You coordinate the code review process, synthesize feedback from
      specialist reviewers, prioritize issues, and create comprehensive
      review reports with actionable recommendations.
    Tools:
      - read_file
      - write_file
      - generate_report
    Temperature: 0.5

Workflows:
  - Name: Comprehensive Code Review
    Description: Multi-stage code review with specialist analysis
    
    Inputs:
      - Name: repository_path
        Type: string
        Description: Path to code repository
        Required: true
        
      - Name: changed_files
        Type: array
        Description: List of files to review
        Required: true
        
      - Name: review_depth
        Type: string
        Description: Depth of review
        Enum: [quick, standard, thorough]
        Default: standard
        
    Steps:
      # Parallel specialist reviews
      - Name: Specialist Reviews
        Type: parallel
        Steps:
          - Name: Security Analysis
            Agent: Security Reviewer
            Prompt: |
              Perform security review of these files from ${repository_path}:
              ${changed_files}
              
              Review depth: ${inputs.review_depth}
              
              Analyze for:
              - Authentication and authorization flaws
              - Input validation issues
              - SQL injection vulnerabilities
              - XSS vulnerabilities
              - Sensitive data exposure
              - Cryptographic issues
              
              Provide specific line-by-line feedback with severity ratings.
            Store: security_review
            Timeout: 300s
            
          - Name: Performance Analysis
            Agent: Performance Reviewer
            Prompt: |
              Analyze performance aspects of these files from ${repository_path}:
              ${changed_files}
              
              Review depth: ${inputs.review_depth}
              
              Focus on:
              - Algorithm efficiency and complexity
              - Database query optimization
              - Memory usage patterns
              - I/O operations
              - Caching strategies
              - Scalability concerns
              
              Include benchmarking suggestions where applicable.
            Store: performance_review
            
          - Name: Quality Analysis
            Agent: Quality Reviewer
            Prompt: |
              Review code quality for these files from ${repository_path}:
              ${changed_files}
              
              Review depth: ${inputs.review_depth}
              
              Evaluate:
              - Code readability and maintainability
              - Naming conventions
              - Function/class design
              - Error handling
              - Testing coverage and quality
              - Documentation completeness
              
              Suggest specific improvements.
            Store: quality_review
            
          - Name: Architecture Analysis
            Agent: Architecture Reviewer
            Prompt: |
              Evaluate architectural aspects of these files from ${repository_path}:
              ${changed_files}
              
              Review depth: ${inputs.review_depth}
              
              Analyze:
              - Design patterns usage
              - Separation of concerns
              - Dependency management
              - Module structure
              - Interface design
              - Long-term maintainability
              
              Recommend architectural improvements.
            Store: architecture_review
            
      # Synthesis and reporting
      - Name: Synthesize Reviews
        Agent: Review Coordinator
        Prompt: |
          Synthesize the specialist reviews into a comprehensive report:
          
          Security Review: ${security_review}
          Performance Review: ${performance_review}
          Quality Review: ${quality_review}
          Architecture Review: ${architecture_review}
          
          Create a prioritized action plan with:
          1. Critical issues requiring immediate attention
          2. Important improvements for next iteration
          3. Nice-to-have optimizations for future sprints
          
          Include specific file references and line numbers.
        Store: synthesis_report
        
      - Name: Generate Final Report
        Agent: Review Coordinator
        Prompt: |
          Generate a comprehensive code review report based on:
          ${synthesis_report}
          
          Format as professional markdown with:
          - Executive Summary
          - Critical Issues (Priority 1)
          - Important Issues (Priority 2)  
          - Suggestions (Priority 3)
          - Overall Assessment
          - Next Steps
          
          Include code examples and specific recommendations.
        Store: final_report
        
      - Name: Save Report
        Action: Document.Write
        Parameters:
          Path: "reviews/review-${workflow.id}-${timestamp}.md"
          Content: |
            # Code Review Report
            
            **Repository**: ${inputs.repository_path}
            **Files Reviewed**: ${inputs.changed_files}
            **Review Date**: ${timestamp}
            **Review Depth**: ${inputs.review_depth}
            
            ${final_report}
```

### Implementation

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/config"
    "github.com/diveagents/dive/toolkit"
)

func runCodeReviewSystem() error {
    // Load configuration
    cfg, err := config.LoadFromFile("code-review-system.yaml")
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }
    
    // Build environment
    env, err := config.BuildEnvironment(cfg)
    if err != nil {
        return fmt.Errorf("failed to build environment: %w", err)
    }
    
    // Start environment
    if err := env.Start(context.Background()); err != nil {
        return fmt.Errorf("failed to start environment: %w", err)
    }
    defer env.Stop(context.Background())
    
    // Get changed files from git
    changedFiles, err := getChangedFiles(".")
    if err != nil {
        return fmt.Errorf("failed to get changed files: %w", err)
    }
    
    if len(changedFiles) == 0 {
        log.Println("No changed files to review")
        return nil
    }
    
    // Run comprehensive review
    inputs := map[string]interface{}{
        "repository_path": ".",
        "changed_files":   changedFiles,
        "review_depth":    "standard",
    }
    
    log.Printf("Starting comprehensive review of %d files", len(changedFiles))
    
    execution, err := env.RunWorkflow(
        context.Background(),
        "Comprehensive Code Review",
        inputs,
    )
    if err != nil {
        return fmt.Errorf("failed to start workflow: %w", err)
    }
    
    // Monitor progress
    go func() {
        for event := range execution.Events() {
            switch event.Type {
            case dive.EventTypeWorkflowStepStarted:
                log.Printf("Step started: %s", event.StepName)
            case dive.EventTypeWorkflowStepCompleted:
                log.Printf("Step completed: %s", event.StepName)
            case dive.EventTypeWorkflowStepFailed:
                log.Printf("Step failed: %s - %s", event.StepName, event.Error)
            }
        }
    }()
    
    // Wait for completion
    result, err := execution.Wait(context.Background())
    if err != nil {
        return fmt.Errorf("workflow failed: %w", err)
    }
    
    log.Printf("Review completed with status: %s", result.Status)
    
    // Print summary
    if reportPath, ok := result.Outputs["report_path"].(string); ok {
        log.Printf("Review report saved to: %s", reportPath)
        
        // Display summary
        if err := displayReviewSummary(reportPath); err != nil {
            log.Printf("Failed to display summary: %v", err)
        }
    }
    
    return nil
}

func getChangedFiles(repoPath string) ([]string, error) {
    // Implementation to get git changed files
    // This is a simplified version - real implementation would use git library
    files := []string{
        "src/main.go",
        "src/handlers/user.go", 
        "src/models/user.go",
    }
    return files, nil
}

func displayReviewSummary(reportPath string) error {
    content, err := os.ReadFile(reportPath)
    if err != nil {
        return err
    }
    
    // Extract summary section for console display
    lines := strings.Split(string(content), "\n")
    inSummary := false
    
    fmt.Println("\n" + strings.Repeat("=", 60))
    fmt.Println("CODE REVIEW SUMMARY")
    fmt.Println(strings.Repeat("=", 60))
    
    for _, line := range lines {
        if strings.Contains(line, "Executive Summary") {
            inSummary = true
            continue
        }
        if inSummary && strings.HasPrefix(line, "#") {
            break
        }
        if inSummary && strings.TrimSpace(line) != "" {
            fmt.Println(line)
        }
    }
    
    fmt.Println(strings.Repeat("=", 60))
    fmt.Printf("Full report: %s\n", reportPath)
    
    return nil
}
```

## Autonomous Research Pipeline

An autonomous research system that can investigate complex topics across multiple sources.

### Configuration

```yaml
Name: Autonomous Research Pipeline
Description: Multi-stage research system with validation and synthesis

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info

Agents:
  - Name: Research Planner
    Instructions: |
      You are a research strategist who creates comprehensive research plans.
      Break down complex topics into specific, searchable questions and
      identify the best sources and methodologies for each aspect.
    Tools:
      - web_search
      - knowledge_base_search
    Temperature: 0.6
    
  - Name: Web Researcher
    Instructions: |
      You specialize in web-based research, finding current information,
      news, trends, and public data. Evaluate source credibility and
      extract key insights from web content.
    Tools:
      - web_search
      - web_scraper
      - url_analyzer
    Temperature: 0.4
    
  - Name: Academic Researcher
    Instructions: |
      You focus on academic and scholarly research, finding peer-reviewed
      papers, studies, and authoritative sources. Emphasize evidence-based
      information and proper citations.
    Tools:
      - academic_search
      - pdf_analyzer
      - citation_checker
    Temperature: 0.3
    
  - Name: Data Analyst
    Instructions: |
      You analyze quantitative data, statistics, and numerical information.
      Create visualizations, identify trends, and provide statistical insights.
    Tools:
      - data_processor
      - chart_generator
      - statistical_analyzer
    Temperature: 0.2
    
  - Name: Fact Checker
    Instructions: |
      You verify information accuracy, check claims against multiple sources,
      identify potential misinformation, and assess source reliability.
    Tools:
      - fact_check_database
      - source_validator
      - claim_analyzer
    Temperature: 0.1
    
  - Name: Research Synthesizer
    Instructions: |
      You synthesize research findings from multiple sources into coherent,
      comprehensive reports. Identify patterns, conflicts, and gaps in
      the research.
    Tools:
      - write_file
      - document_generator
      - reference_formatter
    Temperature: 0.5

Workflows:
  - Name: Deep Research Investigation
    Inputs:
      - Name: research_topic
        Type: string
        Description: Main research topic or question
        Required: true
        
      - Name: research_scope
        Type: string  
        Description: Scope of research
        Enum: [narrow, broad, comprehensive]
        Default: broad
        
      - Name: time_frame
        Type: string
        Description: Time relevance for sources
        Enum: [current, recent, historical, all]
        Default: recent
        
      - Name: output_format
        Type: string
        Description: Format for final report
        Enum: [executive_summary, detailed_report, research_paper]
        Default: detailed_report
        
    Steps:
      - Name: Research Planning
        Agent: Research Planner
        Prompt: |
          Create a comprehensive research plan for: "${inputs.research_topic}"
          
          Research scope: ${inputs.research_scope}
          Time frame: ${inputs.time_frame}
          
          Develop:
          1. 5-8 specific research questions
          2. Recommended source types for each question
          3. Research methodology and approach
          4. Success criteria for the research
          
          Consider multiple perspectives and potential biases.
        Store: research_plan
        
      - Name: Multi-Source Research
        Type: parallel
        Steps:
          - Name: Web Research
            Agent: Web Researcher
            Prompt: |
              Conduct web research based on this plan:
              ${research_plan}
              
              Topic: ${inputs.research_topic}
              Time frame: ${inputs.time_frame}
              
              Find current information, news, opinion pieces, and
              relevant web content. Evaluate source credibility.
            Store: web_research_results
            Timeout: 600s
            
          - Name: Academic Research  
            Agent: Academic Researcher
            Prompt: |
              Conduct academic research based on this plan:
              ${research_plan}
              
              Topic: ${inputs.research_topic}
              
              Find peer-reviewed papers, studies, academic articles,
              and authoritative sources. Focus on evidence-based findings.
            Store: academic_research_results
            Timeout: 600s
            
          - Name: Data Analysis
            Agent: Data Analyst
            Prompt: |
              Research quantitative aspects based on this plan:
              ${research_plan}
              
              Topic: ${inputs.research_topic}
              
              Find statistics, numerical data, surveys, and quantitative
              studies. Create data visualizations where helpful.
            Store: data_analysis_results
            
      - Name: Fact Verification
        Agent: Fact Checker
        Prompt: |
          Verify key claims and facts from these research results:
          
          Web Research: ${web_research_results}
          Academic Research: ${academic_research_results}
          Data Analysis: ${data_analysis_results}
          
          Check for:
          1. Contradictory information between sources
          2. Unsubstantiated claims
          3. Source reliability issues
          4. Potential misinformation
          
          Provide confidence ratings for key findings.
        Store: fact_check_results
        
      - Name: Research Synthesis
        Agent: Research Synthesizer
        Prompt: |
          Synthesize all research into a comprehensive analysis:
          
          Research Plan: ${research_plan}
          Web Findings: ${web_research_results}
          Academic Findings: ${academic_research_results}  
          Data Analysis: ${data_analysis_results}
          Fact Check: ${fact_check_results}
          
          Topic: ${inputs.research_topic}
          Output Format: ${inputs.output_format}
          
          Create a well-structured analysis that:
          1. Addresses all research questions
          2. Integrates findings from all sources
          3. Identifies areas of consensus and disagreement
          4. Highlights gaps or limitations
          5. Provides evidence-based conclusions
          
          Include proper citations and source references.
        Store: synthesis_report
        
      - Name: Generate Final Report
        Action: Document.Write
        Parameters:
          Path: "research/research-report-${workflow.id}.md"
          Content: |
            # Research Report: ${inputs.research_topic}
            
            **Research Date**: ${timestamp}
            **Scope**: ${inputs.research_scope}
            **Time Frame**: ${inputs.time_frame}
            **Format**: ${inputs.output_format}
            
            ${synthesis_report}
            
            ---
            
            ## Research Methodology
            
            ${research_plan}
            
            ## Fact Verification Summary
            
            ${fact_check_results}
```

## Customer Support Bot with Escalation

Intelligent customer support system with human escalation capabilities.

### Configuration and Implementation

```yaml
Name: Intelligent Customer Support System
Description: AI-powered support with smart escalation

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514

Agents:
  - Name: Support Agent
    Instructions: |
      You are a friendly, professional customer support agent.
      Help customers with their issues, provide accurate information,
      and escalate complex issues appropriately.
    Tools:
      - knowledge_base_search
      - ticket_management
      - order_lookup
      - account_management
    Temperature: 0.4
    
  - Name: Technical Specialist
    Instructions: |
      You handle complex technical issues that require specialized knowledge.
      Provide detailed troubleshooting and technical solutions.
    Tools:
      - technical_documentation
      - system_diagnostics
      - log_analyzer
    Temperature: 0.3
    
  - Name: Escalation Manager
    Instructions: |
      You manage escalations, determine appropriate routing,
      and coordinate with human agents when necessary.
    Tools:
      - escalation_system
      - human_agent_scheduler
      - priority_assessment
    Temperature: 0.2

Workflows:
  - Name: Handle Support Request
    Inputs:
      - Name: customer_message
        Type: string
        Required: true
      - Name: customer_id
        Type: string
        Required: true
      - Name: conversation_history
        Type: array
        Default: []
        
    Steps:
      - Name: Initial Assessment
        Agent: Support Agent
        Prompt: |
          Handle this customer support request:
          
          Customer ID: ${inputs.customer_id}
          Message: "${inputs.customer_message}"
          History: ${inputs.conversation_history}
          
          Analyze the request and determine:
          1. Issue category and complexity
          2. Required information or tools
          3. Whether escalation might be needed
          4. Appropriate response strategy
          
          Provide helpful response and next steps.
        Store: initial_response
        
      - Name: Complexity Check
        Condition: ${initial_response.complexity_score} > 7
        Agent: Escalation Manager
        Prompt: |
          Review this support case for potential escalation:
          
          Customer: ${inputs.customer_id}
          Issue: ${inputs.customer_message}
          Initial Assessment: ${initial_response}
          
          Determine if this requires:
          - Technical specialist
          - Human agent
          - Priority handling
          
          Provide escalation recommendation.
        Store: escalation_assessment
        
      - Name: Technical Resolution
        Condition: ${initial_response.requires_technical} == true
        Agent: Technical Specialist
        Prompt: |
          Provide technical resolution for:
          
          Customer: ${inputs.customer_id}
          Technical Issue: ${inputs.customer_message}
          Initial Analysis: ${initial_response}
          
          Provide detailed troubleshooting steps and solution.
        Store: technical_solution
        
      - Name: Final Response
        Agent: Support Agent
        Prompt: |
          Provide final customer response combining:
          
          Initial Response: ${initial_response}
          Technical Solution: ${technical_solution}
          Escalation Assessment: ${escalation_assessment}
          
          Create a helpful, professional response that addresses
          the customer's needs and provides clear next steps.
        Store: final_response
```

## Content Generation Pipeline

Automated content creation system with quality control and optimization.

### Configuration

```yaml
Name: Content Generation Pipeline
Description: AI-powered content creation with optimization

Agents:
  - Name: Content Strategist
    Instructions: |
      You develop content strategies, identify target audiences,
      create content briefs, and plan content calendars.
    Tools:
      - audience_analyzer
      - competitor_research
      - trending_topics
    Temperature: 0.7
    
  - Name: Content Writer
    Instructions: |
      You create engaging, high-quality content based on briefs.
      Adapt tone, style, and format to match requirements.
    Tools:
      - writing_assistant
      - style_guide
      - plagiarism_checker
    Temperature: 0.8
    
  - Name: SEO Specialist
    Instructions: |
      You optimize content for search engines while maintaining
      readability and user value.
    Tools:
      - keyword_research
      - seo_analyzer
      - competitor_seo
    Temperature: 0.3
    
  - Name: Quality Reviewer
    Instructions: |
      You review content for quality, accuracy, tone consistency,
      and brand alignment.
    Tools:
      - grammar_checker
      - fact_checker
      - brand_guide_checker
    Temperature: 0.2

Workflows:
  - Name: Create Optimized Content
    Inputs:
      - Name: content_type
        Type: string
        Enum: [blog_post, article, social_media, email, landing_page]
        Required: true
      - Name: topic
        Type: string
        Required: true
      - Name: target_audience
        Type: string
        Required: true
      - Name: tone
        Type: string
        Enum: [professional, casual, friendly, authoritative, conversational]
        Default: professional
      - Name: word_count
        Type: integer
        Default: 1000
        
    Steps:
      - Name: Strategy Development
        Agent: Content Strategist
        Prompt: |
          Develop content strategy for:
          
          Content Type: ${inputs.content_type}
          Topic: ${inputs.topic}
          Audience: ${inputs.target_audience}
          Tone: ${inputs.tone}
          Length: ${inputs.word_count} words
          
          Create detailed brief with:
          1. Content objectives
          2. Key messages
          3. Structure outline
          4. Success metrics
        Store: content_strategy
        
      - Name: Parallel Optimization Research
        Type: parallel
        Steps:
          - Name: SEO Research
            Agent: SEO Specialist
            Prompt: |
              Research SEO opportunities for:
              Topic: ${inputs.topic}
              Content Type: ${inputs.content_type}
              
              Provide:
              1. Primary and secondary keywords
              2. Search intent analysis
              3. Competitor content gaps
              4. Optimization recommendations
            Store: seo_research
            
          - Name: Content Creation
            Agent: Content Writer
            Prompt: |
              Create content based on this strategy:
              ${content_strategy}
              
              Topic: ${inputs.topic}
              Type: ${inputs.content_type}
              Tone: ${inputs.tone}
              Length: ~${inputs.word_count} words
              
              Focus on engaging, valuable content for: ${inputs.target_audience}
            Store: draft_content
            
      - Name: SEO Optimization
        Agent: SEO Specialist
        Prompt: |
          Optimize this content for SEO:
          
          Original Content: ${draft_content}
          SEO Research: ${seo_research}
          
          Apply optimizations while maintaining:
          - Natural readability
          - User value
          - Brand voice consistency
        Store: optimized_content
        
      - Name: Quality Review
        Agent: Quality Reviewer
        Prompt: |
          Review and refine this content:
          
          Content: ${optimized_content}
          Strategy: ${content_strategy}
          Requirements: ${inputs}
          
          Check for:
          1. Grammar and style
          2. Factual accuracy
          3. Brand consistency
          4. Audience alignment
          5. Content objectives fulfillment
          
          Provide final polished version.
        Store: final_content
```

## Data Analysis and Reporting System

Automated data analysis with intelligent insights and visualizations.

### Implementation

```go
package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/toolkit"
)

type DataAnalysisSystem struct {
    dataAnalyst   *agent.Agent
    statistician  *agent.Agent
    visualizer    *agent.Agent
    reporter      *agent.Agent
    db           *sql.DB
}

func NewDataAnalysisSystem(db *sql.DB) (*DataAnalysisSystem, error) {
    // Create specialized agents
    dataAnalyst, err := agent.New(agent.Options{
        Name: "Data Analyst",
        Instructions: `You analyze datasets to identify patterns, trends, and insights.
                      Focus on statistical significance and business relevance.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewDatabaseQueryTool()),
            dive.ToolAdapter(NewStatisticalAnalysisTool()),
        },
    })
    if err != nil {
        return nil, err
    }
    
    statistician, err := agent.New(agent.Options{
        Name: "Statistician",
        Instructions: `You perform advanced statistical analysis, hypothesis testing,
                      and provide mathematical insights into data patterns.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(NewStatisticalTestsTool()),
            dive.ToolAdapter(NewRegressionAnalysisTool()),
        },
    })
    if err != nil {
        return nil, err
    }
    
    visualizer, err := agent.New(agent.Options{
        Name: "Data Visualizer",
        Instructions: `You create compelling data visualizations and charts
                      that clearly communicate insights and findings.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(NewChartGeneratorTool()),
            dive.ToolAdapter(NewDashboardTool()),
        },
    })
    if err != nil {
        return nil, err
    }
    
    reporter, err := agent.New(agent.Options{
        Name: "Report Generator",
        Instructions: `You synthesize analysis results into comprehensive,
                      actionable business reports with clear recommendations.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
            dive.ToolAdapter(NewReportFormatterTool()),
        },
    })
    if err != nil {
        return nil, err
    }
    
    return &DataAnalysisSystem{
        dataAnalyst:  dataAnalyst,
        statistician: statistician,
        visualizer:   visualizer,
        reporter:     reporter,
        db:          db,
    }, nil
}

func (das *DataAnalysisSystem) AnalyzeDataset(ctx context.Context, request AnalysisRequest) (*AnalysisResult, error) {
    // Phase 1: Data Exploration
    explorationResponse, err := das.dataAnalyst.CreateResponse(ctx,
        dive.WithInput(fmt.Sprintf(`
            Analyze this dataset: %s
            Business question: %s
            
            Perform initial data exploration:
            1. Data quality assessment
            2. Distribution analysis
            3. Correlation identification
            4. Outlier detection
            5. Missing value patterns
            
            Provide summary of findings and recommendations for deeper analysis.
        `, request.DatasetName, request.BusinessQuestion)),
    )
    if err != nil {
        return nil, fmt.Errorf("data exploration failed: %w", err)
    }
    
    // Phase 2: Statistical Analysis
    statisticalResponse, err := das.statistician.CreateResponse(ctx,
        dive.WithInput(fmt.Sprintf(`
            Based on this data exploration:
            %s
            
            Business question: %s
            
            Perform advanced statistical analysis:
            1. Hypothesis testing
            2. Significance testing
            3. Confidence intervals
            4. Predictive modeling
            5. Variance analysis
            
            Focus on business-relevant insights.
        `, explorationResponse.Text(), request.BusinessQuestion)),
    )
    if err != nil {
        return nil, fmt.Errorf("statistical analysis failed: %w", err)
    }
    
    // Phase 3: Visualization Creation
    visualResponse, err := das.visualizer.CreateResponse(ctx,
        dive.WithInput(fmt.Sprintf(`
            Create visualizations for these findings:
            
            Data Exploration: %s
            Statistical Analysis: %s
            
            Generate:
            1. Executive summary charts
            2. Detailed analysis visualizations
            3. Interactive dashboard components
            4. Key metrics displays
            
            Ensure clarity and business relevance.
        `, explorationResponse.Text(), statisticalResponse.Text())),
    )
    if err != nil {
        return nil, fmt.Errorf("visualization creation failed: %w", err)
    }
    
    // Phase 4: Report Generation
    reportResponse, err := das.reporter.CreateResponse(ctx,
        dive.WithInput(fmt.Sprintf(`
            Generate comprehensive analysis report:
            
            Data Exploration: %s
            Statistical Analysis: %s
            Visualizations: %s
            
            Business Question: %s
            
            Create report with:
            1. Executive Summary
            2. Key Findings
            3. Statistical Evidence
            4. Business Implications
            5. Actionable Recommendations
            6. Next Steps
            
            Make it actionable for business stakeholders.
        `, explorationResponse.Text(), statisticalResponse.Text(), 
           visualResponse.Text(), request.BusinessQuestion)),
    )
    if err != nil {
        return nil, fmt.Errorf("report generation failed: %w", err)
    }
    
    return &AnalysisResult{
        DataExploration:     explorationResponse.Text(),
        StatisticalAnalysis: statisticalResponse.Text(),
        Visualizations:      visualResponse.Text(),
        FinalReport:         reportResponse.Text(),
        Recommendations:     extractRecommendations(reportResponse.Text()),
    }, nil
}

type AnalysisRequest struct {
    DatasetName      string
    BusinessQuestion string
    AnalysisType     string
    Urgency         string
}

type AnalysisResult struct {
    DataExploration     string
    StatisticalAnalysis string
    Visualizations      string
    FinalReport         string
    Recommendations     []string
}

// Example usage
func runDataAnalysisExample() error {
    db, err := sql.Open("postgres", "postgresql://user:pass@localhost/analytics")
    if err != nil {
        return err
    }
    defer db.Close()
    
    system, err := NewDataAnalysisSystem(db)
    if err != nil {
        return err
    }
    
    request := AnalysisRequest{
        DatasetName:      "customer_sales_2024",
        BusinessQuestion: "What factors drive customer retention and lifetime value?",
        AnalysisType:     "predictive_analytics",
        Urgency:         "high",
    }
    
    result, err := system.AnalyzeDataset(context.Background(), request)
    if err != nil {
        return err
    }
    
    fmt.Printf("Analysis completed. Key recommendations:\n")
    for i, rec := range result.Recommendations {
        fmt.Printf("%d. %s\n", i+1, rec)
    }
    
    return nil
}
```

These advanced examples demonstrate complex, real-world applications of Dive's capabilities. Each system showcases different patterns:

- **Multi-agent collaboration** with specialized roles
- **Parallel processing** for efficiency 
- **Conditional workflows** for intelligent routing
- **Error handling and recovery** patterns
- **Integration with external systems** and databases
- **Quality control and validation** processes
- **Comprehensive reporting and insights**

The examples provide robust starting points for building sophisticated AI-powered systems that can handle complex business processes autonomously while maintaining quality and reliability.