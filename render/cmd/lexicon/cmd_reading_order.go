package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/lexicon/render/internal/loader"
)

// A readingOrderSource is one candidate primary source for the reading-order
// tab. Matching is by lineage.text PREFIX against curatedSources/sparkCandidates
// below, never by hand-listed atom id -- a new atom mined into an existing
// work shows up automatically on the next `reading-order` run, the same way
// export-graph never needs a manual atom list.
//
// "core" sources are a fixed, hand-picked set (the corpus's pattern-dense
// primary texts). "spark" sources are a generous CANDIDATE pool of
// single/few-atom sources with outsized influence elsewhere in the corpus;
// which of them actually ship is decided by computed tree-reach below, not
// by this list's order.
type readingOrderSource struct {
	Key      string // stable id for this node in the emitted JSON
	Title    string
	Author   string
	Edition  string
	Kind     string // "core" or "spark"
	Prefixes []string
	URL      string // filled in by hand after link verification; empty until then
	// Note is a one-line content note surfaced on the card, for a source
	// whose author or text carries something a reader should know before
	// clicking "read it" -- documented historical/ethical baggage, not an
	// editorial judgment about the work's quality. Empty for nearly every
	// source; see the cringe-check pass, 2026-08-30.
	Note string
}

// matches reports whether lineage text le belongs to this source, comparing
// case-insensitively -- the corpus has stray capitalized slug variants
// (Woolf-1929-..., Spinoza-1677-Ethics-Part-III-..., Hofstadter-2007-
// IAmAStrangeLoop) mixed in with the lowercase convention everything else
// uses.
func (s readingOrderSource) matches(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range s.Prefixes {
		if strings.HasPrefix(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

var curatedSources = []readingOrderSource{
	{Key: "jataka", Title: "The Jataka, or Stories of the Buddha's Former Births", Author: "trans. various, ed. E. B. Cowell", Edition: "Cambridge University Press, 1895–1907, 6 vols", Kind: "core", Prefixes: []string{"jataka-"}, URL: "https://sacred-texts.com/bud/j1/index.htm"},
	{Key: "neihardt-black-elk-speaks", Title: "Black Elk Speaks", Author: "John G. Neihardt", Edition: "1932; Complete Edition, University of Nebraska Press, 2014", Kind: "core", Prefixes: []string{"neihardt-1932-black-elk-speaks-"}, URL: "https://bookshop.org/p/books/black-elk-speaks-john-g-neihardt/65256685272a9d0a"},
	{Key: "merton-social-theory", Title: "Social Theory and Social Structure", Author: "Robert K. Merton", Edition: "Revised and Enlarged Edition, Free Press, 1957", Kind: "core", Prefixes: []string{"merton-1949-social-theory", "merton-1957"}},
	{Key: "aesop-fables", Title: "Aesop's Fables", Author: "trans. V. S. Vernon Jones", Edition: "1912", Kind: "core", Prefixes: []string{"aesop-vernonjones1912-"}, URL: "https://www.gutenberg.org/ebooks/11339"},
	{Key: "cushing-zuni-folk-tales", Title: "Zuñi Folk Tales", Author: "Frank Hamilton Cushing", Edition: "G. P. Putnam's Sons, 1901", Kind: "core", Prefixes: []string{"cushing-"}, URL: "https://www.gutenberg.org/ebooks/54682",
		Note: "Zuni elders documented Cushing acquiring sacred artifacts from them by theft, against their wishes."},
	{Key: "radin-trickster", Title: "The Trickster: A Study in American Indian Mythology", Author: "Paul Radin", Edition: "Philosophical Library, 1956", Kind: "core", Prefixes: []string{"radin-"}},
	{Key: "jackall-moral-mazes", Title: "Moral Mazes: The World of Corporate Managers", Author: "Robert Jackall", Edition: "Oxford University Press, 2014", Kind: "core", Prefixes: []string{"jackall-2014-moral-mazes"}, URL: "https://bookshop.org/books/moral-mazes-the-world-of-corporate-managers-anniversary/9780199729883"},
	{Key: "panchatantra-ryder", Title: "The Panchatantra", Author: "trans. Arthur W. Ryder", Edition: "University of Chicago Press, 1925", Kind: "core", Prefixes: []string{"panchatantra-ryder1925-"}, URL: "https://archive.org/details/ryder-1925-panchatantra-english"},
	{Key: "gilman-women-and-economics", Title: "Women and Economics", Author: "Charlotte Perkins Gilman", Edition: "Small, Maynard & Company, 1898", Kind: "core", Prefixes: []string{"gilman-1898-women-and-economics-"}, URL: "https://www.gutenberg.org/ebooks/57913"},
	{Key: "popper-conjectures", Title: "Conjectures and Refutations: The Growth of Scientific Knowledge", Author: "Karl Popper", Edition: "1963", Kind: "core", Prefixes: []string{"popper-1934-conjectures", "popper-1963-conjectures"}, URL: "https://bookshop.org/p/books/conjectures-and-refutations-the-growth-of-scientific-knowledge-karl-popper/8211365"},
	// Seven case-study chapters (Fox, Goss, Rogoff, Kendall & Selvin, Martin,
	// Huntington, Olencki) from one edited volume -- each got double-counted
	// under two different work-keys by an earlier citation-text-based
	// grouping pass (one exact-slug key, one author-name key derived from
	// the same atoms' citation prose) before this was caught; the real
	// slugs below are the seven distinct lineage.text values, no duplicates.
	{Key: "student-physician", Title: "The Student-Physician: Introductory Studies in the Sociology of Medical Education", Author: "eds. Robert K. Merton, George G. Reader, Patricia L. Kendall", Edition: "Harvard University Press for The Commonwealth Fund, 1957", Kind: "core", Prefixes: []string{"fox-1957", "goss-1957", "rogoff-1957", "kendall-selvin-1957", "martin-1957", "huntington-1957", "olencki-1957"}},
	{Key: "altshuller-triz", Title: "Creativity as an Exact Science: The Theory of the Solution of Inventive Problems", Author: "G. S. Altshuller", Edition: "trans. Anthony Williams, Gordon and Breach, 1984", Kind: "core", Prefixes: []string{"altshuller-1984-"}},
	{Key: "sen-collective-choice", Title: "Collective Choice and Social Welfare", Author: "Amartya Sen", Edition: "Holden-Day, 1970; Expanded Edition, Penguin, 2017", Kind: "core", Prefixes: []string{"sen-1970-collective-choice-and-social-welfare"}},
	{Key: "boethius-consolation", Title: "The Consolation of Philosophy", Author: "Boethius", Edition: "c. 524 AD, trans. H. R. James, 1897", Kind: "core", Prefixes: []string{"boethius-c524-consolation-of-philosophy-"}, URL: "https://www.gutenberg.org/ebooks/14328"},
	{Key: "kjv-bible", Title: "The Holy Bible", Author: "King James Version", Edition: "1611", Kind: "core", Prefixes: []string{"kjv-1611-"}, URL: "https://www.gutenberg.org/ebooks/10"},
	// No verified free copy of the Rangarajan 1992 translation the atoms
	// actually cite -- an earlier link pointed to Shamasastry's 1915
	// translation instead (different wording, different book structure,
	// verses won't resolve against the cited section labels). Leave
	// unlinked rather than link the wrong edition.
	{Key: "kautilya-arthashastra", Title: "The Arthashastra", Author: "Kautilya", Edition: "trans. Rangarajan, 1992", Kind: "core", Prefixes: []string{"kautilya-arthashastra"}},
	{Key: "freeman-strategic-management", Title: "Strategic Management: A Stakeholder Approach", Author: "R. Edward Freeman", Edition: "Pitman, 1984", Kind: "core", Prefixes: []string{"freeman-1984-strategic-management-"}, URL: "https://bookshop.org/p/books/strategic-management-a-stakeholder-approach-r-edward-freeman/7352278"},
	{Key: "gilman-herland", Title: "Herland", Author: "Charlotte Perkins Gilman", Edition: "1915", Kind: "core", Prefixes: []string{"gilman-1915-herland-"}, URL: "https://www.gutenberg.org/ebooks/32",
		Note: "Herland's own prose includes eugenics-inflected passages on breeding out 'the lowest types.'"},
	{Key: "eastman-wigwam-evenings", Title: "Wigwam Evenings: Sioux Folk Tales Retold", Author: "Charles A. Eastman (Ohiyesa) and Elaine Goodale Eastman", Edition: "Little, Brown, 1909", Kind: "core", Prefixes: []string{"eastman-"}, URL: "https://www.gutenberg.org/ebooks/28099"},
	{Key: "walton-reed-macagno-argumentation", Title: "Argumentation Schemes", Author: "Douglas Walton, Chris Reed, Fabrizio Macagno", Edition: "Cambridge University Press, 2008", Kind: "core", Prefixes: []string{"walton-reed-macagno-2008"}},
	{Key: "zitkala-sa-legends", Title: "Old Indian Legends", Author: "Zitkála-Ša", Edition: "Ginn & Company, 1901", Kind: "core", Prefixes: []string{"zitkala-sa"}, URL: "https://www.gutenberg.org/ebooks/338"},
	{Key: "plutarch-lives", Title: "Plutarch's Lives", Author: "Plutarch, trans. John Dryden, rev. Arthur Hugh Clough", Edition: "c. 100 AD; Dryden/Clough translation", Kind: "core", Prefixes: []string{"plutarch-c100-"}, URL: "https://www.gutenberg.org/ebooks/674"},
	{Key: "taylor-36-stratagems", Title: "The Thirty-Six Stratagems: A Modern-Day Interpretation of a Strategy Classic", Author: "Peter Taylor", Edition: "Infinite Ideas, 2013", Kind: "core", Prefixes: []string{"taylor-2013-36-stratagems-"}},
	{Key: "galef-even-monkeys", Title: "Even Monkeys Fall From Trees: And Other Japanese Proverbs", Author: "David Galef, compiler and translator", Edition: "Tuttle Publishing, 1987", Kind: "core", Prefixes: []string{"galef-"}},
	{Key: "legge-i-ching", Title: "The Yi King (I Ching)", Author: "trans. James Legge", Edition: "Sacred Books of the East vol. 16, Oxford University Press, 1899", Kind: "core", Prefixes: []string{"legge-1899-"}, URL: "https://sacred-texts.com/ich/index.htm"},
	{Key: "douglass-life-and-times", Title: "Life and Times of Frederick Douglass", Author: "Frederick Douglass", Edition: "Park Publishing Co., 1881", Kind: "core", Prefixes: []string{"douglass-1881-life-and-times-"}, URL: "https://www.gutenberg.org/ebooks/71893"},
	{Key: "lent-patterning-instinct", Title: "The Patterning Instinct: A Cultural History of Humanity's Search for Meaning", Author: "Jeremy Lent", Edition: "Prometheus, 2017", Kind: "core", Prefixes: []string{"lent-2017-"}, URL: "https://bookshop.org/books/the-patterning-instinct-a-cultural-history-of-humanity-s-search-for-meaning/9781633882935"},
	{Key: "brown-human-universals", Title: "Human Universals", Author: "Donald E. Brown", Edition: "McGraw-Hill, 1991", Kind: "core", Prefixes: []string{"brown-1991-human-universals"}},
	{Key: "taleb-antifragile", Title: "Antifragile: Things That Gain from Disorder", Author: "Nassim Nicholas Taleb", Edition: "Random House, 2012", Kind: "core", Prefixes: []string{"taleb-2012-antifragile", "taleb-lindy-effect"}, URL: "https://bookshop.org/p/books/antifragile-things-that-gain-from-disorder-nassim-nicholas-taleb/5c0b0f56897450d7"},
	{Key: "spinoza-ethics", Title: "Ethica", Author: "Baruch Spinoza", Edition: "1677, posthumous", Kind: "core", Prefixes: []string{"spinoza-1677-ethics-"}, URL: "https://www.gutenberg.org/ebooks/3800"},
	{Key: "homer-iliad", Title: "The Iliad", Author: "Homer", Edition: "trans. William Cullen Bryant, 1870", Kind: "core", Prefixes: []string{"homer-iliad-"}, URL: "https://standardebooks.org/ebooks/homer/the-iliad/william-cullen-bryant"},
	{Key: "heuer-intelligence-analysis", Title: "Psychology of Intelligence Analysis", Author: "Richards J. Heuer Jr.", Edition: "CIA Center for the Study of Intelligence, 1999", Kind: "core", Prefixes: []string{"heuer-1999-"}, URL: "https://bookshop.org/p/books/the-psychology-of-intelligence-analysis-richard-j-heuer/8274836"},
	{Key: "lucretius-de-rerum-natura", Title: "De Rerum Natura (On the Nature of Things)", Author: "Titus Lucretius Carus", Edition: "c. 55 BCE, trans. William Ellery Leonard", Kind: "core", Prefixes: []string{"lucretius-c55bce-de-rerum-natura"}, URL: "https://archive.org/details/bwb_P7-DDE-737"},
	{Key: "franklin-way-to-wealth", Title: "The Way to Wealth", Author: "Benjamin Franklin", Edition: "1758", Kind: "core", Prefixes: []string{"franklin-1758-way-to-wealth"}, URL: "https://www.gutenberg.org/ebooks/43855"},
	{Key: "waite-pictorial-key-tarot", Title: "The Pictorial Key to the Tarot", Author: "Arthur Edward Waite", Edition: "Rider, 1910", Kind: "core", Prefixes: []string{"waite-1910-pictorial-key-"}, URL: "https://sacred-texts.com/tarot/pkt/index.htm"},
	{Key: "mill-system-of-logic", Title: "A System of Logic, Ratiocinative and Inductive", Author: "John Stuart Mill", Edition: "1843", Kind: "core", Prefixes: []string{"mill-1843-"}, URL: "https://www.gutenberg.org/ebooks/27942"},
	// Previous link resolved to a $39.99 out-of-print CD-Audio edition --
	// bookshop.org product IDs can point at the wrong format behind an
	// otherwise-normal-looking slug. Verified 2026-08-30: this one's the
	// hardcover, in stock.
	{Key: "cialdini-influence", Title: "Influence: The Psychology of Persuasion", Author: "Robert Cialdini", Edition: "William Morrow, 1984", Kind: "core", Prefixes: []string{"cialdini-1984-influence"}, URL: "https://bookshop.org/p/books/influence-new-and-expanded-the-psychology-of-persuasion-robert-b-cialdini-phd/c27a82be9a0720a0"},
	{Key: "maurer-big-con", Title: "The Big Con: The Story of the Confidence Man", Author: "David W. Maurer", Edition: "1940", Kind: "core", Prefixes: []string{"maurer-1940-the-big-con"}, URL: "https://bookshop.org/p/books/the-big-con-the-story-of-the-confidence-man-david-maurer/8657182"},
	{Key: "polya-how-to-solve-it", Title: "How to Solve It", Author: "George Pólya", Edition: "Princeton University Press, 1945", Kind: "core", Prefixes: []string{"polya-19"}, URL: "https://bookshop.org/books/how-to-solve-it-a-new-aspect-of-mathematical-method-9780691164076/9780691164076"},
	{Key: "woolf-room-of-ones-own", Title: "A Room of One's Own", Author: "Virginia Woolf", Edition: "1929", Kind: "core", Prefixes: []string{"woolf-1929-aroomofonesown"}, URL: "https://gutenberg.ca/ebooks/woolfv-aroomofonesown/woolfv-aroomofonesown-00-h.html"},
}

// spark candidates: a generous pool of single/few-atom sources with high
// GLOBAL in-degree elsewhere in the corpus. Which ~15 actually ship is
// decided by tree-restricted reach (computeReach below), not by hand.
var sparkCandidates = []readingOrderSource{
	{Key: "wiener-cybernetics", Title: "Cybernetics: Or Control and Communication in the Animal and the Machine", Author: "Norbert Wiener", Edition: "1948", Kind: "spark", Prefixes: []string{"wiener-1948-cybernetics"}, URL: "https://bookshop.org/p/books/cybernetics-or-control-and-communication-in-the-animal-and-the-machine-norbert-wiener/369a287181707110"},
	{Key: "flavell-metacognition", Title: "Metacognition and Cognitive Monitoring", Author: "John H. Flavell", Edition: "American Psychologist, 1979", Kind: "spark", Prefixes: []string{"flavell-1979-metacognition"}, URL: "https://jgregorymcverry.com/readings/flavell1979MetacognitionAndCogntiveMonitoring.pdf"},
	{Key: "turing-computable-numbers", Title: "On Computable Numbers, with an Application to the Entscheidungsproblem", Author: "Alan Turing", Edition: "1936", Kind: "spark", Prefixes: []string{"turing-1936-computable-numbers"}, URL: "https://www.cs.virginia.edu/~robins/Turing_Paper_1936.pdf"},
	{Key: "freeman-tyranny-of-structurelessness", Title: "The Tyranny of Structurelessness", Author: "Jo Freeman", Edition: "1972–73", Kind: "spark", Prefixes: []string{"freeman-1972-tyrannyofstructurelessness"}, URL: "https://www.jofreeman.com/joreen/tyranny.htm"},
	{Key: "reber-schwarz-fluency", Title: "Effects of Perceptual Fluency on Judgments of Truth", Author: "Rolf Reber and Norbert Schwarz", Edition: "Consciousness and Cognition, 1999", Kind: "spark", Prefixes: []string{"reber-schwarz-1999"}, URL: "https://carlo-hamalainen.net/stuff/Reber_Schwarz_Perceptual_fluency.pdf"},
	{Key: "nickerson-confirmation-bias", Title: "Confirmation Bias: A Ubiquitous Phenomenon in Many Guises", Author: "Raymond S. Nickerson", Edition: "Review of General Psychology, 1998", Kind: "spark", Prefixes: []string{"nickerson-1998-confirmation-bias"}, URL: "https://pages.ucsd.edu/~mckenzie/nickersonConfirmationBias.pdf"},
	// http, not https: incompleteideas.net serves a shared Dreamhost SNI
	// placeholder cert (CN sni.dreamhost.com) that doesn't match its own
	// hostname, so https here just trades a working page for a browser
	// security warning. Confirmed by curl 2026-08-30 -- plain http serves
	// the real essay; the fix is on Sutton's hosting, not something we
	// can correct by changing scheme alone on a matching cert.
	{Key: "sutton-bitter-lesson", Title: "The Bitter Lesson", Author: "Richard S. Sutton", Edition: "2019", Kind: "spark", Prefixes: []string{"sutton-2019-bitter-lesson"}, URL: "http://www.incompleteideas.net/IncIdeas/BitterLesson.html"},
	// Previous link resolved to an unavailable CD-Audio edition. Verified
	// 2026-08-30: this one's the paperback, in stock.
	{Key: "kuhn-structure-of-scientific-revolutions", Title: "The Structure of Scientific Revolutions", Author: "Thomas S. Kuhn", Edition: "University of Chicago Press, 1962", Kind: "spark", Prefixes: []string{"kuhn-1962"}, URL: "https://bookshop.org/books/the-structure-of-scientific-revolutions-50th-anniversary-edition/9780226458120"},
	{Key: "goffman-presentation-of-self", Title: "The Presentation of Self in Everyday Life", Author: "Erving Goffman", Edition: "1959", Kind: "spark", Prefixes: []string{"goffman-1959"}, URL: "https://bookshop.org/p/books/the-presentation-of-self-in-everyday-life-erving-goffman/54312506540f2312"},
	{Key: "arthur-increasing-returns", Title: "Increasing Returns and Path Dependence in the Economy", Author: "W. Brian Arthur", Edition: "University of Michigan Press, 1994", Kind: "spark", Prefixes: []string{"arthur-1994-increasing-returns"}},
	// Previous link resolved to a backordered CD-Audio edition; no in-stock
	// US paperback found. Falls back to uk.bookshop.org, same as
	// berlin-hedgehog-and-fox below -- verified 2026-08-30, in stock.
	{Key: "mills-power-elite", Title: "The Power Elite", Author: "C. Wright Mills", Edition: "Oxford University Press, 1956", Kind: "spark", Prefixes: []string{"mills-1956-power-elite"}, URL: "https://uk.bookshop.org/p/books/the-power-elite-c-wright-mills/1448191"},
	{Key: "klein-project-premortem", Title: "Performing a Project Premortem", Author: "Gary Klein", Edition: "Harvard Business Review, 2007", Kind: "spark", Prefixes: []string{"klein-2007"}, URL: "http://homepages.se.edu/cvonbergen/files/2013/01/Performing-a-Project-Premortem.pdf"},
	{Key: "mitchell-russo-pennington-temporal-perspective", Title: "Back to the Future: Temporal Perspective in the Explanation of Events", Author: "Deborah J. Mitchell, J. Edward Russo, Nancy Pennington", Edition: "Journal of Behavioral Decision Making, 1989", Kind: "spark", Prefixes: []string{"mitchell-russo-pennington-1989"}, URL: "https://onlinelibrary.wiley.com/doi/abs/10.1002/bdm.3960020103"},
	{Key: "bataille-erotism", Title: "Erotism: Death and Sensuality", Author: "Georges Bataille", Edition: "1957", Kind: "spark", Prefixes: []string{"bataille-1957-erotism"}, URL: "https://bookshop.org/books/erotism-death-and-sensuality/9780872861909"},
	{Key: "butler-gender-trouble", Title: "Gender Trouble: Feminism and the Subversion of Identity", Author: "Judith Butler", Edition: "Routledge, 1990", Kind: "spark", Prefixes: []string{"butler-1990-gender-trouble"}, URL: "https://bookshop.org/p/books/gender-trouble-feminism-and-the-subversion-of-identity-judith-butler/9053459"},
	{Key: "cooper-voice-from-the-south", Title: "A Voice from the South", Author: "Anna Julia Cooper", Edition: "1892", Kind: "spark", Prefixes: []string{"cooper-1892-avoicefromthesouth"}, URL: "https://www.gutenberg.org/ebooks/61741"},
	{Key: "jung-archetypes", Title: "The Archetypes and the Collective Unconscious", Author: "C. G. Jung", Edition: "Collected Works vol. 9, Princeton/Bollingen, 1959", Kind: "spark", Prefixes: []string{"jung-cw9i-archetypes"}, URL: "https://bookshop.org/p/books/the-archetypes-and-the-collective-unconscious-c-g-jung/9b57e2f7b0752b0c",
		Note: "Jung accepted the presidency of a Nazi-era psychotherapy society in 1933 and wrote of a superior 'Aryan unconscious' in 1934."},
	{Key: "bacon-novum-organum", Title: "Novum Organum", Author: "Francis Bacon", Edition: "1620", Kind: "spark", Prefixes: []string{"bacon-1620-novum-organum"}, URL: "https://www.gutenberg.org/ebooks/45988"},
	{Key: "rosch-natural-categories", Title: "Natural Categories", Author: "Eleanor H. Rosch", Edition: "Cognitive Psychology, 1973", Kind: "spark", Prefixes: []string{"rosch-1973"}, URL: "https://qualquant.org/wp-content/uploads/cda/Rosch%201973%20Natural%20Categories.pdf"},
	{Key: "meadows-thinking-in-systems", Title: "Thinking in Systems: A Primer", Author: "Donella Meadows", Edition: "Chelsea Green, 2008", Kind: "spark", Prefixes: []string{"meadows-2008-thinking-in-systems"}, URL: "https://bookshop.org/p/books/thinking-in-systems-international-bestseller-donella-meadows/8755142"},
	{Key: "nagel-what-is-it-like-to-be-a-bat", Title: "What Is It Like to Be a Bat?", Author: "Thomas Nagel", Edition: "The Philosophical Review, 1974", Kind: "spark", Prefixes: []string{"nagel-1974-what-is-it-like-to-be-a-bat"}, URL: "https://www.sas.upenn.edu/~cavitch/pdf-library/Nagel_Bat.pdf"},
	{Key: "hayek-use-of-knowledge-in-society", Title: "The Use of Knowledge in Society", Author: "Friedrich A. Hayek", Edition: "American Economic Review, 1945", Kind: "spark", Prefixes: []string{"hayek-1945-useofknowledgeinsociety", "hayek-1945-use-of-knowledge-in-society"}, URL: "https://www.econlib.org/library/Essays/hykKnw.html"},
	{Key: "premack-woodruff-theory-of-mind", Title: "Does the Chimpanzee Have a Theory of Mind?", Author: "David Premack and Guy Woodruff", Edition: "Behavioral and Brain Sciences, 1978", Kind: "spark", Prefixes: []string{"premack-woodruff-1978-theory-of-mind"}, URL: "https://philpapers.org/rec/PREDTC-3"},
	{Key: "lorde-masters-tools", Title: "The Master's Tools Will Never Dismantle the Master's House", Author: "Audre Lorde", Edition: "1979", Kind: "spark", Prefixes: []string{"lorde-1979-the-masters-tools"}, URL: "https://theanarchistlibrary.org/library/audre-lorde-the-master-s-tools-will-never-dismantle-the-master-s-house.a4.pdf"},
	{Key: "hooks-feminist-theory", Title: "Feminist Theory: From Margin to Center", Author: "bell hooks", Edition: "South End Press, 1984", Kind: "spark", Prefixes: []string{"hooks-1984-feminist-theory"}, URL: "https://bookshop.org/p/books/feminist-theory-from-margin-to-center-bell-hooks/11024226"},
	{Key: "berlin-hedgehog-and-fox", Title: "The Hedgehog and the Fox: An Essay on Tolstoy's View of History", Author: "Isaiah Berlin", Edition: "1953", Kind: "spark", Prefixes: []string{"berlin-1953-hedgehog"}, URL: "https://uk.bookshop.org/a/10403/9781780223070"},
	{Key: "carse-finite-and-infinite-games", Title: "Finite and Infinite Games: A Vision of Life as Play and Possibility", Author: "James P. Carse", Edition: "Free Press, 1986", Kind: "spark", Prefixes: []string{"carse-1986-finite-and-infinite-games"}, URL: "https://bookshop.org/p/books/finite-and-infinite-games-james-carse/bd091bd7f22106b4"},
	{Key: "hofstadter-godel-escher-bach", Title: "Gödel, Escher, Bach: An Eternal Golden Braid", Author: "Douglas R. Hofstadter", Edition: "Basic Books, 1979", Kind: "spark", Prefixes: []string{"hofstadter-1979-godel-escher-bach"}, URL: "https://bookshop.org/p/books/godel-escher-bach-an-eternal-golden-braid-douglas-r-hofstadter/12389924"},
	{Key: "hofstadter-i-am-a-strange-loop", Title: "I Am a Strange Loop", Author: "Douglas R. Hofstadter", Edition: "Basic Books, 2007", Kind: "spark", Prefixes: []string{"hofstadter-2007"}, URL: "https://bookshop.org/p/books/i-am-a-strange-loop-douglas-r-hofstadter/f4ca6403106c6f64"},
	// machiavelli-1513-the-prince-ch-6 / -ch-17: two atoms, real primary
	// lineage, never registered as a candidate source at all -- so they
	// never entered reach/threshold computation and were invisible even in
	// "Further reading". Found 2026-08-31 when a reader asked why
	// Machiavelli and Marx were missing from the tab.
	{Key: "machiavelli-the-prince", Title: "The Prince", Author: "Niccolò Machiavelli", Edition: "1513", Kind: "spark", Prefixes: []string{"machiavelli-1513-the-prince"}},
	// marx-1867-capital / marx-1867-capital-vol-i-part-viii: same gap.
	// Distinct from the unrelated marx-2022-status-and-culture-* slugs
	// elsewhere in the corpus, which cite W. David Marx's *Status and
	// Culture* (2020) -- a same-surname collision in the citation-text
	// convention, not the same author.
	{Key: "marx-capital", Title: "Capital: A Critique of Political Economy, Volume I", Author: "Karl Marx", Edition: "1867, trans. Moore & Aveling, ed. Engels", Kind: "spark", Prefixes: []string{"marx-1867-capital"}},
	{Key: "marx-engels-communist-manifesto", Title: "The Communist Manifesto", Author: "Karl Marx and Friedrich Engels", Edition: "1848", Kind: "spark", Prefixes: []string{"marx-engels-1848-communist-manifesto"}},
}

func allSources() []readingOrderSource {
	all := make([]readingOrderSource, 0, len(curatedSources)+len(sparkCandidates))
	all = append(all, curatedSources...)
	all = append(all, sparkCandidates...)
	return all
}

// roNode is one source as it will be emitted, after atom matching and
// reach computation have both run.
type roNode struct {
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	Author     string   `json:"author"`
	Edition    string   `json:"edition"`
	Kind       string   `json:"kind"`
	URL        string   `json:"url,omitempty"`
	Note       string   `json:"note,omitempty"`
	AtomCount  int      `json:"atom_count"`
	AtomIDs    []string `json:"atom_ids"`
	Reach      int      `json:"reach"` // distinct OTHER selected nodes whose atoms point at this node's atoms
	TotalIndeg int      `json:"total_in_degree"`
	MaxIndeg   int      `json:"max_in_degree"`
	Tier       int      `json:"tier"`
}

type roEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Weight int    `json:"weight"`
	Kind   string `json:"kind"` // "solid" (strongest parent) or "dashed" (secondary echo)
	// DirectionSource says what actually justified a "solid" edge's
	// direction: "scaffolds" means a real scaffolds-from prerequisite
	// exists between these two sources; "related-tiebreak" means no
	// scaffolds-from edge was found for this pair and the direction is
	// still the old proxy (atom_count/key tiebreak on a symmetric
	// related edge -- see the long comment on edge-building below).
	// Empty for "dashed" edges, which are always related-based echoes.
	DirectionSource string `json:"direction_source,omitempty"`
}

// roFurther is a spark candidate that didn't make the top-*sparkCount* cut
// by tree-restricted reach, but still has real standing in the corpus at
// large (global in-degree) -- surfaced as a separate, unordered "further
// reading" list rather than forced into the tree diagram, which is only
// legible at roughly 15 sparks.
type roFurther struct {
	Key        string `json:"key"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	Edition    string `json:"edition"`
	URL        string `json:"url,omitempty"`
	Note       string `json:"note,omitempty"`
	AtomCount  int    `json:"atom_count"`
	Reach      int    `json:"reach"`
	TotalIndeg int    `json:"total_in_degree"`
	MaxIndeg   int    `json:"max_in_degree"`
}

type readingOrderOutput struct {
	Nodes   []roNode    `json:"nodes"`
	Edges   []roEdge    `json:"edges"`
	Further []roFurther `json:"further"`
}

func cmdReadingOrder(renderDir string, args []string) {
	fl := flag.NewFlagSet("reading-order", flag.ExitOnError)
	out := fl.String("out", "", "output path for the reading-order JSON (default: stdout)")
	sparkCount := fl.Int("sparks", 15, "how many spark candidates to keep, ranked by tree-restricted reach")
	if err := fl.Parse(args); err != nil {
		fatal("parse flags: %s", err)
	}

	elementsDir := os.Getenv("LEXICON_ELEMENTS_DIR")
	if elementsDir == "" {
		elementsDir = filepath.Join(renderDir, loader.DefaultElementsDir)
	}
	pool, err := loader.LoadAll(elementsDir)
	if err != nil {
		fatal("loader: %s", err)
	}

	// Match every atom's lineage entries against every candidate source.
	// One atom can belong to more than one source (an atom citing both
	// Popper and Merton counts toward both).
	atomSources := make(map[string][]string, len(pool)) // atom id -> source keys
	sourceAtoms := make(map[string]map[string]bool)     // source key -> atom id set
	for _, s := range allSources() {
		sourceAtoms[s.Key] = map[string]bool{}
	}
	for id, e := range pool {
		for _, le := range e.Lineage {
			for _, s := range allSources() {
				if s.matches(le.Text) {
					if !sourceAtoms[s.Key][id] {
						sourceAtoms[s.Key][id] = true
						atomSources[id] = append(atomSources[id], s.Key)
					}
				}
			}
		}
	}

	// Global in-degree (for the leverage stats on each node), then
	// tree-restricted reach (for spark selection and diagram edges):
	// only count a citation if the CITING atom also belongs to some
	// source in the candidate universe -- a spark's global fame doesn't
	// matter if nothing else in THIS tree actually leans on it.
	globalIndeg := map[string]int{}
	for _, e := range pool {
		for _, rel := range e.Related {
			globalIndeg[rel]++
		}
	}

	edgeWeight := map[string]map[string]int{} // citer source key -> target source key -> count
	reachSet := map[string]map[string]bool{}  // target source key -> set of distinct citer source keys
	for key := range sourceAtoms {
		edgeWeight[key] = map[string]int{}
		reachSet[key] = map[string]bool{}
	}
	for citerID := range atomSources {
		citerEntry := pool[citerID]
		citerKeys := atomSources[citerID]
		for _, relID := range citerEntry.Related {
			targetKeys, ok := atomSources[relID]
			if !ok {
				continue
			}
			for _, ck := range citerKeys {
				for _, tk := range targetKeys {
					if ck == tk {
						continue // same source citing itself internally isn't a tree edge
					}
					edgeWeight[ck][tk]++
					reachSet[tk][ck] = true
				}
			}
		}
	}

	// Directed scaffolds-from weight, source key -> source key: for every
	// atom X with a populated scaffolds-from list, each primer atom Y
	// contributes one to scaffoldsWeight[source(Y)][source(X)] -- Y primes
	// X, so Y's source is upstream of X's source. Unlike edgeWeight above
	// (built from the symmetric related field, direction decided only by
	// a tiebreak), this is a REAL directional signal wherever it exists.
	// It is necessarily sparser than edgeWeight -- scaffolds-from is only
	// populated on 877 of 3664 atoms as of the 2026-08-30 retroactive
	// pass, and only ever a subset of an atom's related list -- so it's
	// used to override the direction of a solid edge where it has an
	// opinion, not to replace edgeWeight/reachSet as the reach/spark-
	// ranking signal, which stays on the denser related-based numbers.
	scaffoldsWeight := map[string]map[string]int{}
	for key := range sourceAtoms {
		scaffoldsWeight[key] = map[string]int{}
	}
	for primedID, primedKeys := range atomSources {
		primedEntry := pool[primedID]
		for _, primerID := range primedEntry.ScaffoldsFrom {
			primerKeys, ok := atomSources[primerID]
			if !ok {
				continue
			}
			for _, pk := range primerKeys {
				// If the primed atom is ITSELF also attested to the
				// primer's source, this scaffolds-from entry is that
				// atom's own internal composition (decompose-into
				// constituents are definitionally scaffolds-from, per
				// the mint-time gate) showing up as a false cross-source
				// signal, not a real claim that pk's source primes
				// anything else. Confirmed live: lex-9uac6 is cross-
				// attested to both Mill 1843 and Walton-Reed-Macagno
				// 2008, and its scaffolds-from (= its own decompose-into,
				// all Walton atoms) manufactured all 3 weight points of
				// a "Walton primes Mill" edge -- one composed atom's own
				// building blocks, not evidence about the two books.
				primerAlsoPrimed := false
				for _, tk := range primedKeys {
					if pk == tk {
						primerAlsoPrimed = true
						break
					}
				}
				if primerAlsoPrimed {
					continue
				}
				for _, tk := range primedKeys {
					scaffoldsWeight[pk][tk]++
				}
			}
		}
	}

	// Tree-restricted total in-degree per source: how many citing EDGES
	// (not just distinct citing SOURCES, which reachSet already counts)
	// land on this source's atoms, counting only citers that are
	// themselves in the candidate universe. This is the tiebreak for
	// spark selection below -- it must stay tree-restricted, the same as
	// reachSet, or a spark's raw global fame (citations from sources that
	// never make this list at all) would decide close calls instead of
	// how much THIS candidate set actually leans on it.
	treeIndeg := map[string]int{}
	for ck := range edgeWeight {
		for tk, w := range edgeWeight[ck] {
			treeIndeg[tk] += w
		}
	}

	// Rank spark candidates by reach (distinct other nodes leaning on
	// them), tie-broken by tree-restricted in-degree -- never by raw
	// global in-degree, which would let citations from sources outside
	// this candidate set decide a close call.
	type sparkScore struct {
		key   string
		reach int
		total int
	}
	var scored []sparkScore
	for _, s := range sparkCandidates {
		scored = append(scored, sparkScore{key: s.Key, reach: len(reachSet[s.Key]), total: treeIndeg[s.Key]})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].reach != scored[j].reach {
			return scored[i].reach > scored[j].reach
		}
		return scored[i].total > scored[j].total
	})
	keep := map[string]bool{}
	for i, s := range scored {
		if i >= *sparkCount {
			break
		}
		keep[s.key] = true
	}

	// Further reading: spark candidates that didn't make the tree but were
	// actually mined into the corpus (atom_count > 0) -- ranked by global
	// in-degree, since these are by definition NOT tree-restricted-reach
	// winners, so tree reach isn't the right sort key for them.
	var further []roFurther
	for _, s := range sparkCandidates {
		if keep[s.Key] {
			continue
		}
		atomCount := len(sourceAtoms[s.Key])
		if atomCount == 0 {
			continue
		}
		total, max := 0, 0
		for id := range sourceAtoms[s.Key] {
			d := globalIndeg[id]
			total += d
			if d > max {
				max = d
			}
		}
		further = append(further, roFurther{
			Key: s.Key, Title: s.Title, Author: s.Author, Edition: s.Edition, URL: s.URL, Note: s.Note,
			AtomCount: atomCount, Reach: len(reachSet[s.Key]), TotalIndeg: total, MaxIndeg: max,
		})
	}
	sort.Slice(further, func(i, j int) bool {
		if further[i].TotalIndeg != further[j].TotalIndeg {
			return further[i].TotalIndeg > further[j].TotalIndeg
		}
		if further[i].Reach != further[j].Reach {
			return further[i].Reach > further[j].Reach
		}
		return further[i].Key < further[j].Key
	})

	// Final node set: all core sources + the kept sparks.
	bySourceKey := map[string]readingOrderSource{}
	for _, s := range allSources() {
		bySourceKey[s.Key] = s
	}
	var finalKeys []string
	for _, s := range curatedSources {
		finalKeys = append(finalKeys, s.Key)
	}
	for _, s := range sparkCandidates {
		if keep[s.Key] {
			finalKeys = append(finalKeys, s.Key)
		}
	}
	finalSet := map[string]bool{}
	for _, k := range finalKeys {
		finalSet[k] = true
	}

	nodes := make([]roNode, 0, len(finalKeys))
	for _, key := range finalKeys {
		src := bySourceKey[key]
		var ids []string
		for id := range sourceAtoms[key] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		total, max := 0, 0
		for _, id := range ids {
			d := globalIndeg[id]
			total += d
			if d > max {
				max = d
			}
		}
		nodes = append(nodes, roNode{
			Key: key, Title: src.Title, Author: src.Author, Edition: src.Edition,
			Kind: src.Kind, URL: src.URL, Note: src.Note, AtomCount: len(ids), AtomIDs: ids,
			Reach: len(reachSet[key]), TotalIndeg: total, MaxIndeg: max,
		})
	}

	// Edges among the final node set only: each node's single strongest
	// incoming edge is "solid" (its parent in the tree below); its
	// next-strongest incoming related-edges (up to 2 more, weight >= 2)
	// are "dashed" echoes. Keeps the diagram at roughly 1-3 edges per
	// node.
	//
	// A solid edge's DIRECTION now comes from scaffolds-from when one
	// exists between the pair, and only falls back to the old
	// related-based tiebreak otherwise -- see DirectionSource on roEdge.
	//
	// IMPORTANT what the fallback is NOT: `related` is a symmetric field
	// in this corpus (the pre-commit lint gate blocks any unreciprocated
	// edge — see render/cmd/lexicon/cmd_lint_cross_refs.go's
	// unreciprocated-edge check), so "A's atoms point at B's atoms"
	// essentially always means "B's atoms point at A's atoms" too. A
	// real ~35% of solid edges in this feature's first shipped version
	// were exactly this: two sources mutually citing each other at equal
	// weight, "solid parent" decided by nothing more than which key came
	// first in this file (confirmed via an adversarial review
	// 2026-08-29: jataka<->kautilya, bacon<->walton, taleb<->altshuller,
	// and five more). The related-based fallback tiebreak still exists
	// (on the candidate's own atom_count, then key) for pairs scaffolds-
	// from hasn't reached yet -- 877 of 3664 atoms as of 2026-08-30, so
	// most pairs still fall back -- and those edges are marked
	// "related-tiebreak" rather than silently presented the same as a
	// real scaffolds edge. See
	// [[feedback_leverage_over_density_for_curation]] in project memory.
	type inbound struct {
		from   string
		weight int
	}
	sortIns := func(ins []inbound) {
		sort.Slice(ins, func(i, j int) bool {
			if ins[i].weight != ins[j].weight {
				return ins[i].weight > ins[j].weight
			}
			if len(sourceAtoms[ins[i].from]) != len(sourceAtoms[ins[j].from]) {
				return len(sourceAtoms[ins[i].from]) > len(sourceAtoms[ins[j].from])
			}
			return ins[i].from < ins[j].from
		})
	}
	var edges []roEdge
	for _, target := range finalKeys {
		var ins []inbound
		for _, citer := range finalKeys {
			if citer == target {
				continue
			}
			if w := edgeWeight[citer][target]; w > 0 {
				ins = append(ins, inbound{from: citer, weight: w})
			}
		}
		sortIns(ins)

		var scaffoldIns []inbound
		for _, citer := range finalKeys {
			if citer == target {
				continue
			}
			if w := scaffoldsWeight[citer][target]; w > 0 {
				scaffoldIns = append(scaffoldIns, inbound{from: citer, weight: w})
			}
		}
		sortIns(scaffoldIns)

		var solid inbound
		directionSource := "related-tiebreak"
		switch {
		case len(scaffoldIns) > 0:
			solid, directionSource = scaffoldIns[0], "scaffolds"
		case len(ins) > 0:
			solid = ins[0]
		default:
			continue // nothing in the candidate set primes or cites into this target -- a root
		}
		edges = append(edges, roEdge{From: solid.from, To: target, Weight: solid.weight, Kind: "solid", DirectionSource: directionSource})

		dashedCount := 0
		for _, in := range ins {
			if in.from == solid.from {
				continue // already the solid parent, don't double-count it as an echo too
			}
			if dashedCount >= 2 {
				break
			}
			if in.weight >= 2 {
				edges = append(edges, roEdge{From: in.from, To: target, Weight: in.weight, Kind: "dashed"})
				dashedCount++
			}
		}
	}

	// Tier = longest solid-edge path from a root (no solid parent),
	// with a visiting-set cycle guard since solid edges aren't
	// guaranteed acyclic (two nodes can each be the other's strongest
	// parent).
	solidParent := map[string]string{}
	for _, e := range edges {
		if e.Kind == "solid" {
			solidParent[e.To] = e.From
		}
	}
	tier := map[string]int{}
	visiting := map[string]bool{}
	var tierOf func(string) int
	tierOf = func(key string) int {
		if t, ok := tier[key]; ok {
			return t
		}
		parent, hasParent := solidParent[key]
		if !hasParent || !finalSet[parent] {
			tier[key] = 0
			return 0
		}
		if visiting[key] {
			tier[key] = 0 // cycle guard: break here rather than recurse forever
			return 0
		}
		visiting[key] = true
		t := tierOf(parent) + 1
		visiting[key] = false
		tier[key] = t
		return t
	}
	for i := range nodes {
		nodes[i].Tier = tierOf(nodes[i].Key)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Tier != nodes[j].Tier {
			return nodes[i].Tier < nodes[j].Tier
		}
		if nodes[i].AtomCount != nodes[j].AtomCount {
			return nodes[i].AtomCount > nodes[j].AtomCount
		}
		return nodes[i].Key < nodes[j].Key
	})

	output := readingOrderOutput{Nodes: nodes, Edges: edges, Further: further}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fatal("marshal: %s", err)
	}
	if *out == "" {
		os.Stdout.Write(data)
		fmt.Println()
		return
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fatal("mkdir: %s", err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fatal("write: %s", err)
	}
	fmt.Printf("wrote %s (%d nodes, %d edges, %d further, %d spark candidates considered)\n", *out, len(nodes), len(edges), len(further), len(sparkCandidates))
}
