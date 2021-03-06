package md

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/styles"
	"github.com/diamondburned/arikawa/discord"
	"github.com/diamondburned/arikawa/state"
	"github.com/diamondburned/gtkcord3/gtkcord/semaphore"
	"github.com/diamondburned/gtkcord3/log"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

var regexes = []string{
	// codeblock
	`(?:\n?\x60\x60\x60 *(\w*)\n?([\s\S]*?)\n?\x60\x60\x60\n?)`,
	// blockquote
	`((?:(?:^|\n)^>\s+.*)+)\n?`,
	// Inline markup stuff
	`(__|\x60|\*\*\*|\*\*|[_*]|~~|\|\|)`,
	// Hyperlinks
	`<?(https?:\/\S+(?:\.|:)[^>\s]+)>?`,
	// User mentions
	`(?:<@!?(\d+)>)`,
	// Role mentions
	`(?:<@&(\d+)>)`,
	// Channel mentions
	`(?:<#(\d+)>)`,
	// Emojis
	`(<(a?):\w+:(\d+)>)`,
}

var HighlightStyle = "monokai"

var (
	style    = (*chroma.Style)(nil)
	regex    = regexp.MustCompile(`(?m)` + strings.Join(regexes, "|"))
	fmtter   = Formatter{}
	css      = map[chroma.TokenType]string{}
	lexerMap = sync.Map{}
)

type Parser struct {
	pool  sync.Pool
	State *state.State

	ChannelPressed func(ev *gdk.EventButton, ch discord.Channel)
	UserPressed    func(ev *gdk.EventButton, user discord.GuildUser)

	theme *gtk.IconTheme
	icons sync.Map
}

func NewParser(s *state.State) *Parser {
	log.Debugln("REGEX:", strings.Join(regexes, "|"))

	if style == nil {
		style = styles.Get(HighlightStyle)
		if style == nil {
			panic("Unknown highlighting style: " + HighlightStyle)
		}

		css = styleToCSS(style)
	}

	i, err := gtk.IconThemeGetDefault()
	if err != nil {
		// We can panic here, as nothing would work if this ever panics.
		log.Panicln("Couldn't get default GTK Icon Theme:", err)
	}

	p := &Parser{
		State: s,
		theme: i,
	}
	p.pool = newPool(p)

	return p
}

func (p *Parser) GetIcon(name string, size int) *gdk.Pixbuf {
	var key = name + "#" + strconv.Itoa(size)

	if v, ok := p.icons.Load(key); ok {
		return v.(*gdk.Pixbuf)
	}

	pb := semaphore.IdleMust(p.theme.LoadIcon, name, size,
		gtk.IconLookupFlags(gtk.ICON_LOOKUP_FORCE_SIZE)).(*gdk.Pixbuf)

	p.icons.Store(key, pb)
	return pb
}

func (p *Parser) Parse(md []byte, buf *gtk.TextBuffer) {
	p.ParseMessage(nil, nil, md, buf)
}

type Discord interface {
	Channel(discord.Snowflake) (*discord.Channel, error)
	Member(guild, user discord.Snowflake) (*discord.Member, error)
}

func (p *Parser) ParseMessage(d Discord, m *discord.Message, md []byte, buf *gtk.TextBuffer) {
	s := p.pool.Get().(*mdState)
	s.use(buf, md, d, m)

	var tree func(i int)
	if d == nil || m == nil {
		tree = s.switchTree
	} else {
		tree = s.switchTreeMessage
	}

	s.iterMu.Lock()

	// Wipe the buffer clean
	semaphore.IdleMust(func(buf *gtk.TextBuffer) {
		buf.Delete(buf.GetStartIter(), buf.GetEndIter())
	}, buf)

	for i := 0; i < len(s.matches); i++ {
		s.prev = md[s.last:s.matches[i][0].from]
		s.last = s.getLastIndex(i)

		s.insertWithTag(s.prev, nil)
		tree(i)
	}

	s.insertWithTag(md[s.last:], nil)

	// Check if the message is edited:
	if m != nil && m.EditedTimestamp.Valid() {
		s.addEditedStamp(m.EditedTimestamp.Time())
	}

	s.iterMu.Unlock()

	s.iterWg.Wait()

	s.d = nil
	s.m = nil
	s.buf = nil
	s.ttt = nil
	s.tag = nil
	s.last = 0
	s.prev = s.prev[:0]
	s.used = s.used[:0]
	s.hasText = false
	s.attr = 0
	s.color = ""

	p.pool.Put(s)
}
