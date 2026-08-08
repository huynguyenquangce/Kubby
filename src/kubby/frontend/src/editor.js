// editor.js — the YAML editor used everywhere Kubby lets you type a manifest:
// the drawer's YAML tab, Create, Import YAML, and the Helm values boxes.
//
// It replaces a plain <textarea>, which could not do the three things that
// matter for YAML: indent with Tab (the textarea moved focus instead), show line
// numbers (so "document 3 failed" can be pointed at), and colour the text (so a
// wrong indent or a missing colon is visible before the API rejects it).
//
// Colours come from CSS custom properties (--cm-*) rather than a CodeMirror dark
// theme, so light/dark follow the app's existing theme switch with no extra
// wiring. See app.css.

import { EditorState } from '@codemirror/state';
import {
    EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter,
    highlightSpecialChars, drawSelection, dropCursor, rectangularSelection, crosshairCursor,
    placeholder as cmPlaceholder,
} from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import {
    foldGutter, foldKeymap, indentOnInput, indentUnit, bracketMatching,
    syntaxHighlighting, HighlightStyle,
} from '@codemirror/language';
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search';
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';
import { lintGutter, setDiagnostics } from '@codemirror/lint';
import { yaml } from '@codemirror/lang-yaml';
import { tags as t } from '@lezer/highlight';

// Syntax colours as CSS variables: one highlight style serves both themes,
// because the variables themselves change with data-theme.
const yamlHighlight = HighlightStyle.define([
    { tag: [t.definition(t.propertyName), t.propertyName], color: 'var(--cm-key)' },
    { tag: [t.string, t.special(t.string)], color: 'var(--cm-string)' },
    { tag: t.number, color: 'var(--cm-number)' },
    { tag: [t.bool, t.null, t.atom, t.keyword], color: 'var(--cm-atom)' },
    { tag: [t.comment, t.lineComment, t.blockComment], color: 'var(--cm-comment)', fontStyle: 'italic' },
    { tag: [t.meta, t.processingInstruction], color: 'var(--cm-meta)' },
    { tag: t.invalid, color: 'var(--err)' },
]);

const baseTheme = EditorView.theme({
    '&': {
        height: '100%',
        fontSize: '0.82rem',
        backgroundColor: 'var(--cm-bg)',
        color: 'var(--cm-ink)',
    },
    '&.cm-focused': { outline: 'none' },
    '.cm-scroller': {
        fontFamily: 'ui-monospace, "Cascadia Code", Consolas, monospace',
        lineHeight: '1.5',
    },
    '.cm-content': { padding: '0.5rem 0', caretColor: 'var(--cm-ink)' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--cm-ink)' },
    '.cm-gutters': {
        backgroundColor: 'var(--cm-gutter-bg)',
        color: 'var(--cm-gutter-ink)',
        border: 'none',
        borderRight: '1px solid var(--cm-gutter-line)',
    },
    '.cm-activeLine': { backgroundColor: 'var(--cm-active)' },
    '.cm-activeLineGutter': { backgroundColor: 'var(--cm-active)', color: 'var(--cm-ink)' },
    '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
        backgroundColor: 'var(--cm-selection)',
    },
    '.cm-matchingBracket, &.cm-focused .cm-matchingBracket': {
        backgroundColor: 'var(--cm-selection)',
        outline: '1px solid var(--cm-punct)',
    },
    '.cm-placeholder': { color: 'var(--muted)' },
    '.cm-foldPlaceholder': {
        backgroundColor: 'var(--cm-gutter-bg)',
        color: 'var(--muted)',
        border: '1px solid var(--cm-gutter-line)',
    },
});

// createYamlEditor mounts an editor into `host`, which is expected to carry its
// own sizing (see .yaml-host in app.css) — the editor fills it.
export function createYamlEditor(host, { value = '', placeholder = '', onChange } = {}) {
    host.textContent = '';

    const extensions = [
        lineNumbers(),
        highlightActiveLineGutter(),
        highlightSpecialChars(),
        history(),
        foldGutter(),
        drawSelection(),
        dropCursor(),
        EditorState.allowMultipleSelections.of(true),
        indentOnInput(),
        bracketMatching(),
        closeBrackets(),
        rectangularSelection(),
        crosshairCursor(),
        highlightActiveLine(),
        highlightSelectionMatches(),
        lintGutter(), // where markDocumentErrors puts its markers
        syntaxHighlighting(yamlHighlight),
        yaml(),
        // Two spaces is the Kubernetes manifest convention, and YAML has no other
        // legal indent — a literal tab character is a parse error.
        indentUnit.of('  '),
        // Tab indents rather than moving focus. That is the whole point for
        // indentation-sensitive text; CodeMirror's documented escape hatch for
        // keyboard users is Escape first, then Tab, to leave the editor.
        keymap.of([
            indentWithTab,
            ...closeBracketsKeymap,
            ...defaultKeymap,
            ...searchKeymap,
            ...historyKeymap,
            ...foldKeymap,
        ]),
        baseTheme,
    ];
    // No autocompletion: without an OpenAPI schema to complete against it would
    // only ever suggest words already in the buffer, which is noise, not help.
    if (placeholder) extensions.push(cmPlaceholder(placeholder));
    if (onChange) {
        extensions.push(EditorView.updateListener.of((u) => { if (u.docChanged) onChange(); }));
    }

    const view = new EditorView({
        parent: host,
        state: EditorState.create({ doc: value, extensions }),
    });

    return {
        view,
        getValue: () => view.state.doc.toString(),
        setValue(text) {
            view.dispatch({
                changes: { from: 0, to: view.state.doc.length, insert: text ?? '' },
            });
            clearMarks(view);
        },
        focus: () => view.focus(),
        destroy: () => view.destroy(),
        clearMarks: () => clearMarks(view),
        /**
         * markDocumentErrors puts a red gutter marker on the first line of each
         * document that failed, so "document 3 failed" becomes something you can
         * see instead of something you count lines to find.
         *
         * The API error names the object, not a line, so the marker lands on the
         * document's opening line — which is as precise as the server's answer is.
         *
         * @param {Array<{doc: number, message: string}>} errors 1-based document numbers
         */
        markDocumentErrors(errors) {
            const doc = view.state.doc;
            const starts = documentStarts(doc.toString());
            const diagnostics = [];
            for (const e of errors ?? []) {
                const lineIdx = starts[(e.doc ?? 1) - 1] ?? 0;
                const line = doc.line(Math.min(lineIdx + 1, doc.lines));
                diagnostics.push({
                    from: line.from,
                    to: line.to,
                    severity: 'error',
                    message: e.message,
                });
            }
            view.dispatch(setDiagnostics(view.state, diagnostics));
        },
    };
}

function clearMarks(view) {
    view.dispatch(setDiagnostics(view.state, []));
}

// documentStarts returns the 0-based line index at which each YAML document
// begins, numbered the way the backend numbers them: split on a `---` line, and
// skip a segment that holds nothing but whitespace.
//
// Keeping the two in step is what makes "document 3" mark the right line. The
// backend rule lives in splitYAMLDocuments (apply.go).
function documentStarts(text) {
    const lines = text.split('\n');
    const starts = [];
    let atStart = true;
    for (let i = 0; i < lines.length; i++) {
        if (/^---\s*(#.*)?$/.test(lines[i])) { atStart = true; continue; }
        if (atStart && lines[i].trim() !== '') { starts.push(i); atStart = false; }
    }
    return starts;
}

// parseApplyFailures pulls the failed document numbers out of the apply report so
// they can be marked in the editor.
//
// This reads a format produced by ApplyYAML in apply.go ("FAILED   document 3:
// …"). If that line ever changes shape, the markers quietly stop appearing —
// the report itself is still shown in full, so nothing is hidden.
export function parseApplyFailures(report) {
    const out = [];
    for (const line of String(report ?? '').split('\n')) {
        const m = /^FAILED\s+document (\d+):\s*(.*)$/.exec(line);
        if (m) out.push({ doc: Number(m[1]), message: m[2] });
    }
    return out;
}
