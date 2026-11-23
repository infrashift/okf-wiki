// The okf block is what okfw read from each concept's frontmatter: OKF v0.3
// lifecycle, trust and provenance, plus the links okfw resolved, so the page
// can show them without re-implementing the format in the browser.
import { defineCollection, z } from 'astro:content';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';

const link = z.object({ title: z.string(), href: z.string() });

const okf = z
  .object({
    path: z.string(),
    type: z.string().optional(),
    tags: z.array(z.string()).default([]),
    status: z.string().default('stable'),
    stale: z.boolean().default(false),
    staleAfter: z.string().optional(),
    trust: z.string().default('unverified'),
    generatedBy: z.string().optional(),
    generatedAt: z.string().optional(),
    verified: z.array(z.object({ by: z.string(), at: z.string(), current: z.boolean() })).default([]),
    supersededBy: link.optional(),
    pathRefs: z.array(z.object({ field: z.string(), value: z.string(), href: z.string().optional() })).default([]),
    backlinks: z.array(link).default([]),
  })
  .optional();

export const collections = {
  docs: defineCollection({
    loader: docsLoader(),
    schema: docsSchema({ extend: z.object({ okf }) }),
  }),
};
