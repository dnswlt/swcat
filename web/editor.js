import { EditorView, basicSetup } from 'codemirror';
import { yaml } from '@codemirror/lang-yaml';
import { keymap } from '@codemirror/view';
import { indentWithTab } from '@codemirror/commands';

export function initEditor() {
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
