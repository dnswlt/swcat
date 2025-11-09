import { EditorView } from 'codemirror';
import { basicSetup } from 'codemirror';
import { yaml } from '@codemirror/lang-yaml';
import { json } from '@codemirror/lang-json';
import { keymap } from '@codemirror/view';
import { indentWithTab } from '@codemirror/commands';

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
