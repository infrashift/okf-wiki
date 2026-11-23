---
title: Why OKF Wiki?
description: The thinking behind OKF Wiki and why this tool exists.
---

OKF Wiki is a local-first knowledge base in files. It uses the [Open Knowledge Format (OKF)](https://github.com/open-knowledge-format/open-knowledge-format/blob/main/SPEC.md). Each idea is in its own markdown file, a **concept**, with structured frontmatter. Plain markdown links connect the concepts. OKF Wiki uses an idea from the Zettelkasten method: knowledge grows faster when many links connect small, atomic notes.

## The Problem with Traditional Note-Taking

Most note systems have basic problems:

**Folders make you choose a category too early.** When you file a note under "Work > Projects > Design", the note stays in that one context. But many ideas are part of more than one field. Where do you put a note about "psychology in user interface design"? Psychology? Design? User research?

**Linear notes become graveyards.** You take detailed notes in a meeting or from a book. You file them, and you never look at them again. The information exists, but you cannot get it back.

**Search is not enough.** Full-text search helps you find what you remember. It cannot show you a connection between ideas that you forgot, or that you never made.

## The Principles

### 1. Atomic Concepts

Each concept contains one idea. It is not a summary of a chapter or the minutes of a meeting. It is one concept that can stand alone.

**Bad:** "Notes from chapter 5 of a book about decisions"
**Good:** "Fast judgments replace a hard question with an easier one"

Atomic concepts are:
- Easier to link, because you know the idea that you link to
- Easier to find, because one idea is one search target
- Easier to use again, in different contexts

### 2. Links Over Folders

Do not put notes into folders by category. Connect them with explicit links. For example, a concept about "cognitive biases in job interviews" can link to:
- "the bias to look for proof of what we think"
- "structured interviews reduce bias"
- "fast and slow modes of thought"

The links make a network of knowledge, where new ideas come from connections that you did not expect.

### 3. Your Own Words

Do not copy a quote without a note about what it means to you. When you write an idea in different words, you learn it. The knowledge base must contain the thoughts of its author about ideas, not only the ideas.

### 4. Capture, Refine, Link

**Capture quickly.** Put each idea or quote into a concept when it occurs. These concepts are raw material, not finished products.

**Refine regularly.** Look at recent captures again and make them clear: one idea in each concept, a title that states the idea, and no copied text.

**Link actively.** Connect each refined concept to the concepts that it relates to. When you refine and link a capture, you must think about it.

## Why This Tool?

`okfw` (OKF Wiki) follows these principles:

### Lives With Your Project

The wiki is a `.okfw/` directory in the project, so the notes are in the same repository as the code that they describe. A note about "JWT refresh token patterns" is next to the auth code that it explains. Directories put related notes together, and tags connect notes across directories.

### Enforced Structure

The CUE schema makes sure that each concept has:
- A unique id, so links stay correct
- A title, so you state the idea clearly
- A type: the kind of concept, with a name that you choose
- Tags, for categories across directories
- Timestamps for when okfw created it and when it last changed

This structure is not bureaucracy. It is the minimum metadata that a network of knowledge needs.

### Parent-Child Relationships

The optional `x-okfw-parent` field connects a concept to the concept that it develops. A chain of these concepts records how an idea grew, and each concept stays atomic.

### Plain Text Markdown

The concepts are Markdown files in an open format. There is no proprietary format and no database. The files work with each text editor and each sync service. People can read them in 50 years.

## The Compound Effect

The value of a linked knowledge base grows over time.

With 10 concepts, it is a note app.
With 100 concepts, you start to see useful connections.
With 1,000 concepts, it becomes a partner for thought.
With 10,000 concepts, it is an external memory that shows you ideas that you cannot find in a different way.

When you query a knowledge base that you maintain well, you get more than what you put in. You get new connections, and old ideas come back at the right time.

## Getting Started

1. **Start small.** Do not import old notes. Start with new ideas when they occur.

2. **Capture freely.** Create a concept for each idea that you want to keep. Do not judge or filter when you capture.

3. **Refine regularly.** Set a daily or weekly time. Then look at recent captures and make the valuable ones clear.

4. **Link actively.** When you refine a concept, ask: "Which concepts does this connect to?" Then add explicit links.

5. **Trust the process.** The value grows over months and years, not days.

## Further Reading

- *How to Take Smart Notes* by Sönke Ahrens: a guide to linked, atomic notes for knowledge work
- *Building a Second Brain* by Tiago Forte: a digital method to manage personal knowledge

---

The goal is not a perfect note system. The goal is better thought. OKF Wiki helps you make connections, develop ideas over time, and build a body of knowledge that grows in value.

Start with one concept. Then add another. Let the network grow.
