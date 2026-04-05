package tax

import (
	"context"
	"fmt"
)

const transitionSeriesMinYear = 2026
const transitionSeriesMaxYear = 2033

// TransitionSeries executa Calculate para cada ano de 2026 a 2033 com o mesmo
// payload (serviços, despesas, regime), variando apenas Year. Usado para o
// gráfico de transição temporal sem múltiplas chamadas HTTP.
func TransitionSeries(ctx context.Context, eng Engine, base SimulationInput) ([]SimulationResult, error) {
	n := transitionSeriesMaxYear - transitionSeriesMinYear + 1
	out := make([]SimulationResult, 0, n)
	for y := transitionSeriesMinYear; y <= transitionSeriesMaxYear; y++ {
		in := base
		in.Year = y
		r, err := eng.Calculate(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("transition series year %d: %w", y, err)
		}
		out = append(out, r)
	}
	return out, nil
}
