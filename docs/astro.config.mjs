// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// The canonical site is GitHub Pages, served from a project sub-path.
// `base` keeps every internal link and asset URL prefixed correctly.
// Both are overridable via env (e.g. DOCS_BASE=/ for a root-served mirror)
// so switching hosts is a CI change, not a code change.
export default defineConfig({
	site: process.env.DOCS_SITE ?? 'https://lmgarret.github.io',
	base: process.env.DOCS_BASE ?? '/polar-flow-mcp',
	integrations: [
		starlight({
			title: 'polar-flow-mcp',
			description:
				'Single-user MCP server that drives the reverse-engineered Polar Flow web API from Claude.',
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/lmgarret/polar-flow-mcp',
				},
			],
			editLink: {
				baseUrl: 'https://github.com/lmgarret/polar-flow-mcp/edit/main/docs/',
			},
			lastUpdated: true,
			// Diátaxis: Tutorials / How-to Guides / Reference / Explanation.
			sidebar: [
				{
					label: 'Tutorials',
					items: [{ autogenerate: { directory: 'tutorials' } }],
				},
				{
					label: 'How-to Guides',
					items: [{ autogenerate: { directory: 'guides' } }],
				},
				{
					label: 'Reference',
					items: [{ autogenerate: { directory: 'reference' } }],
				},
				{
					label: 'Explanation',
					items: [{ autogenerate: { directory: 'explanation' } }],
				},
			],
		}),
	],
});
