// robots.txt — статический endpoint. Строка Sitemap подставляется,
// только когда в astro.config.mjs вписан реальный домен (siteReady).
import type { APIRoute } from 'astro';
import { siteReady } from '../lib/seo';

export const GET: APIRoute = ({ site }) => {
  const lines = ['User-agent: *', 'Allow: /'];
  if (siteReady && site) {
    lines.push('', `Sitemap: ${site.toString().replace(/\/$/, '')}/sitemap.xml`);
  }
  lines.push('');
  return new Response(lines.join('\n'), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
};