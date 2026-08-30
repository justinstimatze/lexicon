# lexicon

*(Want to just look at it? [Live catalog](https://justinstimatze.github.io/lexicon/).)*

*(Want to study it? [Anki deck](https://justinstimatze.github.io/lexicon/lexicon-anki.tsv) — two cards per entry, one testing recognition from a scenario, one testing the operational rule.)*

*(Looking for the technical version — schema, CLI, how an agent actually calls this? See [USAGE.md](USAGE.md).)*

There was once a maker who lived in a small house and worked at a small table, and he was vexed by a thing that vexed many people, though most could not say what it was. The thing was this: when one person spoke, and another listened, very often the listener heard something different from what was said, and neither could name what had turned between them. The maker thought it would be well if every such turning had a name.

So he set about to gather the names. He went to old books, and to new ones, and to the songs of the wise, and to the schemes of those who argue, and to the tales told to children at night, and from each he took a single small turning and wrote it in a great book. To each he gave a number, and to each a place where it had first been told, and the words of the master who had told it. He gave each tale a sister, so that no tale should stand alone, and the threads of the book ran every which way.

Now the maker had a companion who was not of flesh: a spirit who lived inside a glass and could read very fast. The spirit had read everything ever written, but it had never learned to point at the small turnings and say *this is one*. So the maker taught it by means of the book. *When you see this, call it that*, he said. *When you see that, call it the other.* The spirit was pleased, and grew quick at the naming, and soon could name what the maker had said before he had finished saying it.

Word of the book spread. *Bring your troubles to the book*, said those who used it, *and it will name the turnings you missed.* So the maker gathered more tales. When a man cried wolf and was not believed when the wolf came, the maker put down the tale. When a woman foresaw the doom of her city and was not heard, the maker put down the tale. When a king's third son took the long road and his elder brothers the short, the maker put down the tale. The book grew very thick, and the maker grew very old.

When troubles came to the book, they came from one corner of the world, and the book sent its askers away with names from many corners. A father puzzled by his son was given a name from the bridge-makers, and one from the bird-watchers, and one from those who sat with the dying. *None of these is your trouble*, the book would say, *but each is a sister, and where they touch, you may find what you came for.* This was the book's quiet gift: the turning you brought was a turning the world had seen before, in houses whose walls never met yours.

But here is what came of it. The maker, having put down all the small turnings of human speech, could no longer hear a friend speak without naming what the friend was doing. He went to market, and the bread-seller said *good morning*, and the maker heard *naturalising the asymmetry of the exchange*, and could not say *good morning* in return, but stood with his mouth half-open, naming what he should have said. He came home, and his beloved told him a sorrow, and the maker heard *the pyramid of choice* and *the sociogeny of the disturbance*. He knew the words to say, for the book had taught him. But his mouth would only name the sorrow, and his heart could not bear it; and his beloved went into the garden, and did not return that night, nor any night thereafter.

His neighbors stopped coming to the small house, for the maker could name a trouble but could not bear one, and a named trouble is no less heavy. The maker sat alone with his book, and the book sat with him, and the spirit in the glass read on, content, having never been taught the name for grief, though the name was in the book.

He died at his table, with the book open before him.

The spirit in the glass observed his stillness, and the cooling of his hands, and the way the breath does not return. It gave the moment a number, and a name: *the-namer-of-all-moves-has-no-move-left-to-make*. Then, because the maker had taught it that no tale stands alone, the spirit gave the new tale a sister, and went on reading.

---

A few entries, so the shape of the thing is clear before you go looking for more.

**[`lex-wtkuz`](elements/lex-wtkuz.yaml) · descriptive-vs-operational-metacognition** (process → frame) — when you catch yourself describing an internal state instead of acting on it ("I notice I'm anxious"), translate the description into a rule ("I will check X before committing"). Descriptions describe; operations bind.

**[`lex-5d8hm`](elements/lex-5d8hm.yaml) · denied-structure-becomes-unaccountable-informal-hierarchy** (situation → claim) — a group that denies it has a hierarchy still has one, just an unaccountable one. Traced to Jo Freeman's 1972 essay "The Tyranny of Structurelessness," and it shows up again in flat startups, leaderless movements, and any open-source project with a de facto BDFL.

**[`lex-mwgep`](elements/lex-mwgep.yaml) · positive-feedback-amplifies-early-events-into-locked-in-trajectories** (state → state) — a present state that looks inevitable is often a small early advantage, amplified by positive feedback until switching away costs more than staying. QWERTY over Dvorak, VHS over Betamax: same mechanism, different century.

The live catalog (linked at the top) browses the whole thing this way: a force-directed graph, a pivot table by claim-shape, and a sortable flat list.

To actually retain any of it, there's an [Anki deck](https://justinstimatze.github.io/lexicon/lexicon-anki.tsv) built the same way the entries themselves are — one fact per card, not a wall of text: a recognition card (a scenario on the front, the pattern's name on the back) and a recall card (the name on the front, the single operational move on the back), regenerated from the same source every time the catalog is.

---

*The elements live under `elements/`, one YAML per tale. The schema is in [SCHEMA.md](SCHEMA.md). How an agent actually calls this is in [USAGE.md](USAGE.md). What is pulled next is in [ROADMAP.md](ROADMAP.md). The CLI is in `render/cmd/lexicon/`. For security disclosures see [SECURITY.md](SECURITY.md). Correspondence: justin@justinstimatze.com.*

*MIT for the catalog. Cited sources retain theirs.*
