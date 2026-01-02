import { EditorView } from 'codemirror';
import { basicSetup } from 'codemirror';
import { yaml, yamlLanguage } from '@codemirror/lang-yaml';
import { json } from '@codemirror/lang-json';
import { keymap } from '@codemirror/view';
import { indentWithTab } from '@codemirror/commands';

async function fetchCompletions(field) {
    try {
        const response = await fetch(`/catalog/autocomplete?field=${field}`);
        if (!response.ok) return [];
        return await response.json();
    } catch (e) {
        console.error(e);
        return [];
    }
}

const yamlCompleter = async (context) => {
    const { state, pos } = context;
    const doc = state.doc;
    const currentLine = doc.lineAt(pos);
    const textBeforeCursor = currentLine.text.slice(0, pos - currentLine.from);

    // Guard: Only complete the first word (to avoid confusing popups when adding @v1 or "labels").
    if (/[\w:].* .*/.test(textBeforeCursor)) {
        return null;
    }

    // Get the generic YAML path (e.g., ["metadata", "annotations"])
    const path = getYamlPath(doc, pos);
    const field = path.join(".");

    if (![
        "metadata.annotations",
        "metadata.labels",
        "spec.consumesApis",
        "spec.providesApis",
        "spec.dependsOn"].includes(field)) {
        // Unsupported field, avoid calling the backend.
        return null;
    }

    const completionResponse = await fetchCompletions(field);
    if (!completionResponse) {
        return null;
    }
    const { fieldType, completions } = completionResponse;
    if (!completions) {
        return null;
    }

    // Identify starting point
    const word = context.matchBefore(/(- *)?[\w\.\-\/]*/);
    const from = word ? word.from : pos;

    // Build completions.
    // For lists, the completion includes the "- ", so codemirror does not
    // abort the completion once it sees "- " and doesn't find it in the completion list.
    const isList = fieldType == "item";
    const isKey = fieldType == "key";
    return {
        from: from,
        options: completions.map(c => ({
            label: (isList ? "- " : "") + c,
            type: "property", // defines the icon displayed in the dropdown
            apply: (isList ? "- " : "") + c + (isKey ? ": " : "")
        })),
        validFor: /^(- *)?[\w\.\-\/]*$/  // To only call this function once while in the same word.
    };
};

/**
 * Scans upwards to reconstruct the property path.
 * Handles standard indentation AND compact lists (where item indentation == key indentation).
 */
function getYamlPath(doc, pos) {
    const path = [];
    const currentLine = doc.lineAt(pos);

    // 1. Setup Start Conditions
    let currentIndent = 0;
    let isListMode = false;

    if (currentLine.text.trim().length === 0) {
        // Case: Empty line (user pressed Enter)
        // Use cursor column for indentation
        currentIndent = pos - currentLine.from;

        // If we are on an empty line, we must look at the previous line 
        // to see if we are inside a list.
        isListMode = isPrecedingLineList(doc, currentLine.number);
    } else {
        // Case: Existing line
        currentIndent = getIndent(currentLine.text);
        isListMode = currentLine.text.trim().startsWith("-");
    }

    // 2. Scan Upwards
    for (let i = currentLine.number - 1; i >= 1; i--) {
        const line = doc.line(i);
        const text = line.text;

        // Skip empty lines and comments
        if (!text.trim() || text.trim().startsWith("#")) continue;

        const indent = getIndent(text);

        // CASE A: Standard Parent (Strictly less indented)
        if (indent < currentIndent) {
            const key = getKeyName(text);
            if (key) {
                path.unshift(key);
                currentIndent = indent;
                // Update mode: if this parent is a list item, we are now in list mode
                isListMode = text.trim().startsWith("-");
            }
        }
        // CASE B: Compact List Parent (Equal indentation)
        // ONLY valid if we are in list mode
        else if (indent === currentIndent && isListMode) {
            // A parent cannot start with dash (that would be a sibling list item)
            if (!text.trim().startsWith("-")) {
                const key = getKeyName(text);
                if (key) {
                    path.unshift(key);
                    currentIndent = indent;
                    isListMode = false; // Exited list context
                }
            }
        }
    }
    return path;
}

/**
 * Helper: checks if the closest previous non-empty line starts with "-"
 */
function isPrecedingLineList(doc, startLine) {
    for (let i = startLine - 1; i >= 1; i--) {
        const text = doc.line(i).text.trim();
        if (!text || text.startsWith("#")) continue;
        return text.startsWith("-");
    }
    return false;
}

function getIndent(text) {
    return text.search(/\S|$/);
}

function getKeyName(text) {
    const match = text.match(/^\s*(?:-\s*)?([\w\.\-]+):/);
    return match ? match[1] : null;
}

const yamlAutoCompleteExtension = yamlLanguage.data.of({
    autocomplete: yamlCompleter
});

// Initializes the CodeMirror EditorView for the YAML editor.
export function initYamlEditor() {
    const editorEl = document.getElementById("yaml-editor");
    if (!editorEl) {
        return;
    }

    const editorView = new EditorView({
        doc: editorEl.value,
        extensions: [
            basicSetup,
            yaml(),
            yamlAutoCompleteExtension,
            keymap.of([indentWithTab]),
            // On any update, sync the textarea's value.
            EditorView.updateListener.of((update) => {
                if (update.docChanged) {
                    editorEl.value = editorView.state.doc.toString();
                }
            })
        ],
        parent: editorEl.parentElement
    });
    editorEl.style.display = "none";
}

// Initializes the CodeMirror EditorView to display read-only JSON.
export function initJsonViewer(elementId) {
    const viewerEl = document.getElementById(elementId);
    if (!viewerEl) {
        return;
    }

    new EditorView({
        doc: viewerEl.value,
        extensions: [
            basicSetup,
            json(),
            EditorView.editable.of(false),
            EditorView.lineWrapping,
        ],
        parent: viewerEl.parentElement,
    });
    viewerEl.style.display = "none";
}
