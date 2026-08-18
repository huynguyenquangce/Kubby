import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const indexHTML = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const mainJS = readFileSync(new URL('./main.js', import.meta.url), 'utf8');
const responsiveCSS = readFileSync(new URL('./responsive.css', import.meta.url), 'utf8');

test('narrow shell keeps the canonical sidebar reachable', () => {
    assert.match(indexHTML, /<aside id="sidebar" class="sidebar">/);
    assert.match(indexHTML, /id="btn-mobile-nav"[^>]*aria-controls="sidebar"[^>]*aria-expanded="false"/);
    assert.match(indexHTML, /id="mobile-nav-backdrop"[^>]*hidden/);
    assert.match(mainJS, /function setMobileNavOpen\(open\)/);
    assert.match(mainJS, /setMobileNavOpen\(false\);\s*currentView = view/);
    assert.match(mainJS, /stopImmediatePropagation\(\);\s*setMobileNavOpen\(false\)/);
    assert.match(mainJS, /document\.querySelector\('\.main'\)\.inert = shouldOpen/);
    assert.match(mainJS, /querySelector\('\.nav-item\.active, select, button'\)\?\.focus\(\)/);
    assert.match(responsiveCSS, /\.dashboard\.mobile-nav-open \.sidebar/);
    assert.match(responsiveCSS, /@media \(max-width: 820px\)/);
});

test('topbar labels have shrink targets without removing their actions', () => {
    for (const id of ['btn-create', 'btn-import']) {
        const button = indexHTML.match(new RegExp(`<button id="${id}"[\\s\\S]*?<\\/button>`))?.[0] ?? '';
        assert.match(button, /aria-label="[^"]+"/);
        assert.match(button, /class="topbar-action-label"/);
    }
    assert.match(responsiveCSS, /\.topbar-action-label,[\s\S]*?display: none/);
    assert.doesNotMatch(responsiveCSS, /#btn-(?:create|import)\s*\{[^}]*display:\s*none/);
});

test('component breakpoints use the remaining workspace width', () => {
    assert.match(responsiveCSS, /container-name:\s*workspace/);
    assert.match(responsiveCSS, /container-type:\s*inline-size/);
    for (const width of ['1030px', '960px', '760px', '520px']) {
        assert.match(responsiveCSS, new RegExp(`@container workspace \\(max-width: ${width}\\)`));
    }
    assert.match(responsiveCSS, /@container workspace \(max-width: 760px\)[\s\S]*?\.structure-path\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\)/);
    assert.match(responsiveCSS, /@container workspace \(max-width: 760px\)[\s\S]*?\.term-controls\s*\{[\s\S]*?grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\)/);
    assert.match(responsiveCSS, /@container workspace \(max-width: 520px\)[\s\S]*?\.term-brand > span/);
    assert.match(responsiveCSS, /@container workspace \(max-width: 960px\)[\s\S]*?\.overview-grid/);
});

test('small overlays stay within the viewport and preserve reduced motion', () => {
    assert.match(responsiveCSS, /@media \(max-width: 620px\)[\s\S]*?\.drawer\s*\{\s*width:\s*100vw/);
    assert.match(responsiveCSS, /\.modal\s*\{[\s\S]*?width:\s*calc\(100vw - 1rem\)/);
    assert.match(responsiveCSS, /@media \(prefers-reduced-motion: reduce\)/);
    assert.match(responsiveCSS, /@media \(max-width: 620px\)[\s\S]*?\.modal-form-grid\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\)/);
    assert.match(responsiveCSS, /@media \(max-width: 420px\)[\s\S]*?\.modal-foot-actions \.btn/);
});

test('Helm workspace stacks summaries, forms, and action rows at narrow widths', () => {
    assert.match(responsiveCSS, /@container workspace \(max-width: 960px\)[\s\S]*?\.helm-summary/);
    assert.match(responsiveCSS, /@container workspace \(max-width: 760px\)[\s\S]*?\.helm-catalog-search/);
    assert.match(responsiveCSS, /@container workspace \(max-width: 520px\)[\s\S]*?\.helm-workspace-tabs/);
    assert.match(responsiveCSS, /@container workspace \(max-width: 520px\)[\s\S]*?\.helm-res-row/);
});
