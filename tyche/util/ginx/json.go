package ginx

const (
	ESCAPE   = 92
	QUOTE    = 34
	SPACE    = 32
	TAB      = 9
	NEWLINE  = 10
	ASTERISK = 42
	SLASH    = 47
	HASH     = 35
)

func removeComments(s []byte) []byte {
	var (
		i       int
		quote   bool
		escaped bool
	)
	j := make([]byte, len(s))
	comment := &commentData{}
	for _, ch := range s {
		if ch == ESCAPE || escaped {
			if !comment.startted {
				j[i] = ch
				i++
			}
			escaped = !escaped
			continue
		}
		if ch == QUOTE {
			quote = !quote
		}
		if (ch == SPACE || ch == TAB) && !quote {
			continue
		}
		if ch == NEWLINE {
			if comment.isSingleLined {
				comment.stop()
			}
			continue
		}
		if quote && !comment.startted {
			j[i] = ch
			i++
			continue
		}
		if comment.startted {
			if ch == ASTERISK && !comment.isSingleLined {
				comment.canEnd = true
				continue
			}
			if comment.canEnd && ch == SLASH && !comment.isSingleLined {
				comment.stop()
				continue
			}
			comment.canEnd = false
			continue
		}
		if comment.canStart && (ch == ASTERISK || ch == SLASH) {
			comment.start(ch)
			continue
		}
		if ch == SLASH {
			comment.canStart = true
			continue
		}
		if ch == HASH {
			comment.start(ch)
			continue
		}
		j[i] = ch
		i++
	}
	return j[:i]
}

type commentData struct {
	canStart      bool
	canEnd        bool
	startted      bool
	isSingleLined bool
}

func (c *commentData) stop() {
	c.startted = false
	c.canStart = false
}

func (c *commentData) start(ch byte) {
	c.startted = true
	c.isSingleLined = ch == SLASH || ch == HASH
}
