package main

// func main() {
// 	s := Scanner{}
// 	s.stack = []Mode{ModeRoot}
// 	fp, err := os.Open("../../../dotfiles/dotfiles.json")
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer fp.Close()
// 	s.scanner = bufio.NewScanner(fp)

// 	for s.Scan() {
// 		fmt.Printf("%v at %d:%d-%d\n", s.Token, s.Linenr, s.Start, s.End)
// 	}
// 	if s.Token != TokenEOF {
// 		fmt.Printf("error %v at %d:%d-%d\n", s.Token, s.Linenr, s.Start, s.End)
// 	}
// }
