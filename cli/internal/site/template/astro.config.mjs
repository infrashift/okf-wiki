// Generated site for an OKF bundle, written by `okfw site`. okfw writes
// okfw-site.json (title, description and sidebar) and src/content/docs/ from
// the bundle on every build; this file only wires them into Starlight.
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import site from './okfw-site.json' with { type: 'json' };

export default defineConfig({
  integrations: [
    starlight({
      title: site.title,
      description: site.description || undefined,
      customCss: ['./src/styles/okfw.css'],
      components: {
        PageTitle: './src/components/PageTitle.astro',
      },
      sidebar: site.sidebar,
      pagination: false,
      lastUpdated: false,
    }),
  ],
});
