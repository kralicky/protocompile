package ast

func startToken(nodes ...Node) Token {
	for _, n := range nodes {
		if !IsNil(n) {
			return n.Start()
		}
	}
	return TokenError
}

func endToken(nodes ...Node) Token {
	for _, n := range nodes {
		if !IsNil(n) {
			return n.End()
		}
	}
	return TokenError
}
