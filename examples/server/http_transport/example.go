package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/server"
	"github.com/modelcontextprotocol-ce/go-sdk/server/stream"
	"github.com/modelcontextprotocol-ce/go-sdk/spec"
)

// SimpleToolHandler is a basic tool handler function
func SimpleToolHandler(ctx context.Context, params []byte) (interface{}, error) {
	// Parse parameters
	var p map[string]interface{}
	if params != nil {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
	}

	// Get input from parameters if available
	input, _ := p["input"].(string)
	if input == "" {
		input = "world"
	}

	// Return result
	return map[string]interface{}{
		"message": fmt.Sprintf("Hello, %s! Current time is %s", input, time.Now().Format(time.RFC3339)),
	}, nil
}

// SimpleMessageHandler creates a response to a message
func SimpleMessageHandler(ctx context.Context, request spec.CreateMessageRequest) (*spec.CreateMessageResponse, error) {
	content := request.Content
	if content == "" {
		content = "No question provided"
	}

	// Create a simulated AI response
	result := &spec.CreateMessageResult{
		Role:    "assistant",
		Content: generateResponse(content),
	}

	return &spec.CreateMessageResponse{Result: result}, nil
}

// generateResponse creates a sample response based on the input
func generateResponse(input string) string {
	responses := map[string]string{
		"Explain quantum computing and its real-world applications in detail": `Quantum computing is a revolutionary computational paradigm that leverages the principles of quantum mechanics to process information. Unlike classical computers that use bits (0s and 1s), quantum computers use quantum bits or "qubits" that can exist in multiple states simultaneously thanks to superposition.

Key concepts in quantum computing include:

1. Superposition: Qubits can exist in multiple states at once, enabling quantum computers to process vast amounts of possibilities simultaneously.

2. Entanglement: Qubits can be correlated in ways that have no classical analogue, allowing changes to one qubit to instantly affect another regardless of distance.

3. Quantum interference: This allows quantum algorithms to amplify correct answers and suppress incorrect ones.

Real-world applications of quantum computing include:

1. Cryptography: Quantum computers could break current encryption standards, but they also enable quantum cryptography, offering theoretically unbreakable encryption.

2. Drug discovery and material science: Quantum computers can simulate molecular and chemical interactions with unprecedented accuracy, potentially revolutionizing pharmaceutical development and material design.

3. Optimization problems: Complex optimization problems in logistics, finance, and manufacturing could be solved far more efficiently with quantum approaches.

4. Machine learning: Quantum machine learning algorithms have the potential to identify patterns in data exponentially faster than classical algorithms.

5. Weather forecasting and climate modeling: The enhanced computational power could dramatically improve our ability to model complex climate systems.

Though still in its early stages, quantum computing has already demonstrated "quantum advantage" for specific problems, where quantum computers outperform the most powerful classical supercomputers. Companies like IBM, Google, Microsoft, and D-Wave are actively developing quantum hardware, while countries worldwide are investing billions in quantum research.

Challenges remain in scaling up quantum systems, reducing error rates, and developing practical applications, but the field continues to advance rapidly, with potentially transformative implications across numerous industries in the coming decades.`,

		"Tell me about artificial intelligence": `Artificial Intelligence (AI) refers to the simulation of human intelligence in machines programmed to think and learn like humans. The core objective of AI is to create systems that can function intelligently and independently.

Key areas of AI include:

1. Machine Learning: Algorithms that improve through experience without being explicitly programmed.

2. Natural Language Processing: Enabling computers to understand, interpret, and generate human language.

3. Computer Vision: Allowing machines to identify and process objects, scenes, and activities in images or video.

4. Robotics: Creating autonomous machines that can navigate and interact with their environment.

5. Expert Systems: Programs designed to mimic human expertise in specific domains.

AI has transformative applications across numerous industries, from healthcare (diagnostic tools, personalized medicine) to finance (fraud detection, algorithmic trading), transportation (autonomous vehicles), education (personalized learning), and entertainment (recommendation systems).

The field has experienced dramatic growth in recent years due to advances in computational power, availability of vast datasets, and breakthroughs in deep learning algorithms. However, AI also raises important ethical considerations regarding privacy, bias, job displacement, and autonomous decision-making.

As AI continues to evolve, the focus is increasingly on developing systems that are not only intelligent but also transparent, fair, and aligned with human values and needs.`,
	}

	// Return a predefined response if available, otherwise a generic one
	if response, exists := responses[input]; exists {
		return response
	}

	return "Thank you for your message. This is a simulated response from the MCP server example."
}

func main() {
	fmt.Println("=== HTTP Server Transport with SSE Streaming Example ===")

	// Create the HTTP server transport provider
	addr := "localhost:8080"
	transportProvider := stream.NewHTTPServerTransportProvider(
		addr,
		stream.WithServerDebug(true),
	)

	// Create a server using the builder pattern
	s := server.NewSync(transportProvider).
		WithRequestTimeout(30 * time.Second).
		WithServerInfo(spec.Implementation{
			Name:    "Go MCP HTTP Server",
			Version: "1.0.0",
		}).
		WithCapabilities(spec.NewServerCapabilitiesBuilder().
			Tools(true, true).
			Sampling(true).
			Build()).
		WithTools(
			spec.Tool{
				Name:        "hello-tool",
				Description: "A simple hello world tool",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}}}`),
			},
		).(server.SyncBuilder).
		Build()

	// Register handlers
	s.RegisterToolHandler("hello-tool", SimpleToolHandler)
	s.SetCreateMessageHandler(SimpleMessageHandler)

	// Start the server
	fmt.Printf("Starting MCP server on %s...\n", addr)
	err := s.Start()
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	fmt.Printf("Server started. Connect to http://%s/jsonrpc\n", addr)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for termination signal
	sig := <-sigChan
	fmt.Printf("\nReceived signal %v, shutting down...\n", sig)

	// Gracefully stop the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.StopGracefully(ctx)
	fmt.Println("Server stopped")
}
