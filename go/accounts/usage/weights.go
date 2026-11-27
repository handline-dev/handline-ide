package usage

// WeekWeights is how much each account's week counts when the accounts' weeks are read together: its weekly plan budget, the weight
// TotalOf gives it. An account with no budget counts for nothing, as in TotalOf; with no budget at all every account counts the same.
type WeekWeights map[string]float64

// WeightsOf is the weights of bs: each account with a weekly budget, by that budget.
func WeightsOf(bs []AccountBudget) WeekWeights {
	w := WeekWeights{}
	for _, b := range bs {
		if b.WeekUSD > 0 {
			w[b.Account] = b.WeekUSD
		}
	}
	return w
}

// accountLeft is one account's week left at a point, in percent.
type accountLeft struct {
	Account string
	Left    float64
}

// of is a's weight.
func (w WeekWeights) of(a string) float64 {
	if len(w) == 0 {
		return 1
	}
	return w[a]
}

// Mean is the weighted mean of left; false when no account in it counts.
func (w WeekWeights) Mean(left []accountLeft) (float64, bool) {
	var sum, total float64
	for _, l := range left {
		k := w.of(l.Account)
		sum, total = sum+k*l.Left, total+k
	}
	if total <= 0 {
		return 0, false
	}
	return sum / total, true
}

// MeanOf is Mean over the accounts' values, an account with nil left out.
func (w WeekWeights) MeanOf(accounts []string, left []*float64) (float64, bool) {
	var ls []accountLeft
	for i, a := range accounts {
		if left[i] != nil {
			ls = append(ls, accountLeft{a, *left[i]})
		}
	}
	return w.Mean(ls)
}
