---
title: Why OKF Wiki?
description: The thinking behind OKF Wiki and why this tool exists.
---

OKF Wiki is a local-first, file-based knowledge base built on the [Open Knowledge Format (OKF)](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md). Every idea lives in its own markdown file — a **concept** — with structured frontmatter, and concepts are woven together with plain markdown links. OKF Wiki draws on the Zettelkasten method's insight that knowledge compounds when atomic notes are densely linked.

## The Problem with Traditional Note-Taking

Most note-taking systems suffer from fundamental flaws:

**Folder hierarchies force premature categorization.** When you file a note under "Work > Projects > Marketing", you've locked it into one context. But ideas often span multiple domains. Where do you put a note about "using psychology principles in marketing copy"? Psychology? Marketing? Writing?

**Linear notes become graveyards.** We've all experienced it: you take detailed notes in a meeting or while reading a book, file them away, and never look at them again. The information exists but it's effectively lost.

**Search isn't enough.** Full-text search helps you find what you remember, but can't help you discover connections between ideas you've forgotten or never consciously made.

## The Principles

### 1. Atomic Concepts

Each concept contains exactly one idea. Not a summary of a chapter, not meeting minutes - one discrete concept that can stand alone.

**Bad:** "Notes from Chapter 5 of Thinking Fast and Slow"
**Good:** "System 1 thinking substitutes easier questions for hard ones"

Atomic concepts are:
- Easier to link (you know exactly what you're linking to)
- Easier to find (one idea = one search target)
- More reusable (combine in different contexts)

### 2. Links Over Folders

Instead of organizing notes into folders, you connect them through explicit links. A concept about "cognitive biases in hiring" might link to:
- "confirmation bias definition"
- "structured interviews reduce bias"
- "System 1 vs System 2 thinking"

This creates a network of knowledge where insights emerge from unexpected connections.

### 3. Your Own Words

Never copy-paste quotes without adding your own interpretation. The act of reformulating ideas in your own words is where learning happens. Your knowledge base should contain your thoughts about ideas, not just the ideas themselves.

### 4. Capture, Refine, Link

**Capture quickly.** Ideas that occur to you, interesting quotes, observations - get them into a concept the moment they appear. They're raw material, not finished products.

**Refine regularly.** Revisit recent captures and sharpen them: one idea per concept, a title that crystallizes it, your own words.

**Link actively.** Connect each refined concept to the concepts it relates to. The discipline of refining and linking what you've captured forces you to actually think about it.

## Why This Tool?

`okfw` (OKF Wiki) is built around these principles:

### Lives With Your Project

The wiki is a `.okfw/` directory inside your project — notes travel with the code they describe, in the same repository. Your note about "JWT refresh token patterns" sits next to the auth code it explains, and an optional `--project` tag adds cross-cutting metadata when you want it.

### Enforced Structure

The CUE schema validation ensures every concept has:
- A unique id (for reliable linking)
- A title (forces you to crystallize the idea)
- A type (what kind of concept it is, in your own vocabulary)
- Tags (flexible cross-cutting categorization)
- Created and last-changed timestamps (temporal context)

A project is optional metadata on top of these.

This structure isn't bureaucracy - it's the minimum viable metadata for a functioning knowledge network.

### Parent-Child Relationships

The optional `parent` field supports concept sequences - chains of thought where one idea directly develops from another. This preserves the logical progression of your thinking while maintaining atomic concepts.

### Plain Text Markdown

Your concepts are just Markdown files in an open format. No proprietary format, no database lock-in. They work with any text editor, sync with any service, and will be readable in 50 years.

## The Compound Effect

The real power of a linked knowledge base emerges over time.

With 10 concepts, it's just a note-taking app.
With 100 concepts, you start seeing useful connections.
With 1,000 concepts, it becomes a thinking partner.
With 10,000 concepts, it's an external brain that surfaces insights you couldn't have found any other way.

When you query a well-maintained knowledge base, you don't just get back what you put in. You get emergent connections, forgotten insights resurfacing at the right moment, and the accumulated wisdom of your past thinking.

## Getting Started

1. **Start small.** Don't try to import your existing notes. Begin with new ideas as they occur to you.

2. **Capture freely.** Create a concept for anything interesting. Don't judge or filter at capture time.

3. **Refine regularly.** Set a daily or weekly time to review recent captures and sharpen the valuable ones.

4. **Link actively.** When refining a concept, always ask: "What existing concepts does this connect to?" Add explicit links.

5. **Trust the process.** The value compounds over months and years, not days.

## Further Reading

- *How to Take Smart Notes* by Sönke Ahrens - The definitive guide to linked, atomic note-taking for knowledge work
- *Building a Second Brain* by Tiago Forte - A modern digital adaptation of personal knowledge management

---

The goal isn't to have a perfect note-taking system. It's to think better. OKF Wiki is a tool that augments your natural thinking process, helping you make connections, develop ideas over time, and build a body of knowledge that compounds in value.

Start with one concept. Then another. Let the network grow.
