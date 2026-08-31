package rank

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"time"
)

// MLP is a tiny pure-Go Feed-Forward Neural Network (Multi-Layer Perceptron)
// used to learn the optimal fusion of BM25 and Semantic Similarity scores.
type MLP struct {
	W1 [][]float64 // Weights: Input (3) -> Hidden (4)
	B1 []float64   // Biases: Hidden (4)
	W2 [][]float64 // Weights: Hidden (4) -> Output (1)
	B2 []float64   // Biases: Output (1)
}

// NewMLP initializes a 3 -> 4 -> 1 network with random weights (He initialization).
func NewMLP() *MLP {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	
	mlp := &MLP{
		W1: make([][]float64, 3),
		B1: make([]float64, 4),
		W2: make([][]float64, 4),
		B2: make([]float64, 1),
	}

	// He initialization for W1 (ReLU)
	std1 := math.Sqrt(2.0 / 3.0)
	for i := 0; i < 3; i++ {
		mlp.W1[i] = make([]float64, 4)
		for j := 0; j < 4; j++ {
			mlp.W1[i][j] = rng.NormFloat64() * std1
		}
	}

	// Xavier initialization for W2 (Sigmoid)
	std2 := math.Sqrt(2.0 / (4.0 + 1.0))
	for i := 0; i < 4; i++ {
		mlp.W2[i] = make([]float64, 1)
		mlp.W2[i][0] = rng.NormFloat64() * std2
	}

	return mlp
}

// relu activation
func relu(x float64) float64 {
	if x > 0 {
		return x
	}
	return 0
}

// relu derivative
func dRelu(x float64) float64 {
	if x > 0 {
		return 1
	}
	return 0
}

// sigmoid activation
func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// sigmoid derivative
func dSigmoid(x float64) float64 {
	s := sigmoid(x)
	return s * (1.0 - s)
}

// Forward pass through the network
func (m *MLP) Forward(inputs []float64) float64 {
	if len(inputs) != 3 {
		return 0
	}
	
	// Hidden layer (size 4)
	hidden := make([]float64, 4)
	for j := 0; j < 4; j++ {
		sum := m.B1[j]
		for i := 0; i < 3; i++ {
			sum += inputs[i] * m.W1[i][j]
		}
		hidden[j] = relu(sum)
	}

	// Output layer (size 1)
	outSum := m.B2[0]
	for j := 0; j < 4; j++ {
		outSum += hidden[j] * m.W2[j][0]
	}
	return sigmoid(outSum)
}

// Train executes a mini-batch Gradient Descent backpropagation algorithm.
func (m *MLP) Train(inputs [][]float64, targets []float64, epochs int, lr float64) {
	n := len(inputs)
	if n == 0 || n != len(targets) {
		return
	}

	for epoch := 0; epoch < epochs; epoch++ {
		for s := 0; s < n; s++ {
			x := inputs[s]
			y := targets[s]

			// --- Forward Pass ---
			// Hidden layer pre-activation
			z1 := make([]float64, 4)
			a1 := make([]float64, 4)
			for j := 0; j < 4; j++ {
				z1[j] = m.B1[j]
				for i := 0; i < 3; i++ {
					z1[j] += x[i] * m.W1[i][j]
				}
				a1[j] = relu(z1[j])
			}

			// Output layer pre-activation
			z2 := m.B2[0]
			for j := 0; j < 4; j++ {
				z2 += a1[j] * m.W2[j][0]
			}
			a2 := sigmoid(z2)

			// --- Backward Pass (Backpropagation) ---
			// Loss derivative (Mean Squared Error): dL/da2
			dL_da2 := 2.0 * (a2 - y) / float64(n)

			// Output layer gradients
			da2_dz2 := dSigmoid(z2)
			delta2 := dL_da2 * da2_dz2

			dW2 := make([]float64, 4)
			for j := 0; j < 4; j++ {
				dW2[j] = a1[j] * delta2
			}
			dB2 := delta2

			// Hidden layer gradients
			delta1 := make([]float64, 4)
			for j := 0; j < 4; j++ {
				delta1[j] = delta2 * m.W2[j][0] * dRelu(z1[j])
			}

			dW1 := make([][]float64, 3)
			for i := 0; i < 3; i++ {
				dW1[i] = make([]float64, 4)
				for j := 0; j < 4; j++ {
					dW1[i][j] = x[i] * delta1[j]
				}
			}
			dB1 := delta1

			// --- Update Weights ---
			for j := 0; j < 4; j++ {
				m.W2[j][0] -= lr * dW2[j]
			}
			m.B2[0] -= lr * dB2

			for i := 0; i < 3; i++ {
				for j := 0; j < 4; j++ {
					m.W1[i][j] -= lr * dW1[i][j]
				}
			}
			for j := 0; j < 4; j++ {
				m.B1[j] -= lr * dB1[j]
			}
		}
	}
}

// Save serializes the MLP to disk.
func (m *MLP) Save(path string) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadMLP loads the MLP from disk.
func LoadMLP(path string) (*MLP, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m MLP
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
