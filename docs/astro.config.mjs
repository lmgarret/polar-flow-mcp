// @ts-check
import { defineConfig } from 'astro/config';
import { unified } from '@astrojs/markdown-remark';
import starlight from '@astrojs/starlight';
import astroD2 from 'astro-d2';

// Served as a GitHub Pages project site under /polar-flow-mcp/, so every
// root-absolute link in the content must carry the base prefix.
export default defineConfig({
	site: process.env.DOCS_SITE ?? 'https://lmgarret.github.io',
	base: '/polar-flow-mcp',
	markdown: {
		// Make D2's baked-in dark palette follow Starlight's in-page theme toggle
		// instead of only the OS `prefers-color-scheme`.
		processor: unified({ rehypePlugins: [rehypeD2DarkMode] }),
	},
	integrations: [
		starlight({
			title: 'polar-flow-mcp',
			description:
				'Single-user MCP server that drives the reverse-engineered Polar Flow web API from Claude.',
			logo: {
				src: './src/assets/logo.svg',
				alt: 'polar-flow-mcp',
			},
			favicon: '/favicon.svg',
			customCss: ['./src/styles/theme.css'],
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
		astroD2({
			inline: true, // embed the SVG in the page HTML
			layout: 'elk', // nicer layouts than the default 'dagre'
			pad: 20,
			theme: { default: '0', dark: '200' }, // light / dark D2 theme IDs
		}),
	],
});

// astro-d2 (via the D2 binary) emits the dark palette behind
// `@media screen and (prefers-color-scheme:dark)`, so diagrams would otherwise
// only track the OS theme. This rehype pass rescopes those rules to Starlight's
// `:root[data-theme='dark']` selector so they follow the site's theme toggle.
// Self-contained (no external deps) so it can be lifted verbatim.
function rehypeD2DarkMode() {
	return (tree) => {
		const walk = (node) => {
			if (
				typeof node.value === 'string' &&
				node.value.includes('prefers-color-scheme:dark')
			) {
				node.value = rescopeD2DarkMode(node.value);
			}
			if (Array.isArray(node.children)) node.children.forEach(walk);
		};
		walk(tree);
	};
}

function rescopeD2DarkMode(css) {
	const marker = '@media screen and (prefers-color-scheme:dark){';
	let out = '';
	let i = 0;
	for (;;) {
		const start = css.indexOf(marker, i);
		if (start === -1) {
			out += css.slice(i);
			return out;
		}
		out += css.slice(i, start);
		// Walk balanced braces to find the end of the media block.
		let depth = 1;
		let k = start + marker.length;
		const bodyStart = k;
		for (; k < css.length && depth > 0; k++) {
			if (css[k] === '{') depth++;
			else if (css[k] === '}') depth--;
			if (depth === 0) break;
		}
		const body = css.slice(bodyStart, k);
		// Prefix each rule selector so it only applies under the dark toggle.
		out += body.replace(
			/([^{}]+)\{/g,
			(_m, sel) => `:root[data-theme='dark'] ${sel.trim()}{`,
		);
		i = k + 1; // skip the media block's closing brace
	}
}
