import {Node, mergeAttributes} from '@tiptap/core';
import type {JSONContent} from '@tiptap/core';

export const conversationReferenceType = 'conversationReference';

const escapeHtml = (value: string) => value.replace(/[&<>"']/g, character => ({
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}[character]!));

export const ConversationReference = Node.create({
  name: conversationReferenceType,
  group: 'inline',
  inline: true,
  atom: true,
  selectable: true,
  draggable: false,
  addAttributes() {
    return {
      sessionId: {default: ''},
      title: {default: ''},
      projectName: {default: ''},
      updatedAt: {default: ''},
    };
  },
  parseHTML() {
    return [{tag: 'span[data-conversation-reference]'}];
  },
  renderHTML({HTMLAttributes}) {
    return ['span', mergeAttributes(HTMLAttributes, {
      'data-conversation-reference': HTMLAttributes.sessionId,
      class: 'agent-conversation-reference',
      title: `引用对话：${HTMLAttributes.title}`,
    }), `@${HTMLAttributes.title}`];
  },
  renderText({node}) {
    return `@${node.attrs.title}`;
  },
  renderMarkdown(node: JSONContent) {
    return `@${escapeHtml(String(node.attrs?.title || ''))}`;
  },
});

export interface ComposerReference {
  sessionId: string;
  title: string;
  projectName?: string;
  updatedAt?: string;
}

export function referenceLabel(title: string) {
  return '@' + title;
}
