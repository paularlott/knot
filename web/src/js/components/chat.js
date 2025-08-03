import Alpine from 'alpinejs';

// Simple markdown processor for chat messages
function processMarkdown(text) {
  if (!text) return '';

  return text
    .trim()
    // Code blocks (```language\ncode\n```)
    .replace(/```(\w+)?\n([\s\S]*?)\n```/g, (match, lang, code) => {
      const language = lang || 'text';
      return `<pre class="bg-gray-100 dark:bg-gray-900 dark:border dark:border-gray-700 p-3 rounded-md overflow-x-auto my-2"><code class="language-${language} text-sm">${escapeHtml(code.trim())}</code></pre>`;
    })
    // Inline code (`code`)
    .replace(/`([^`]+)`/g, '<code class="bg-gray-100 dark:bg-gray-900 dark:border dark:border-gray-700 px-1 py-0.5 rounded text-sm font-mono">$1</code>')
    // Block quotes (&gt; text)
    .replace(/^((?:>\s*.+(?:\n|$))+)/gm, (match) => {
      const lines = match.split('\n').filter(line => line.trim());
      const content = lines.map(line => line.replace(/^>\s?/, '')).join('\n');
      return `<blockquote class="border-l-4 border-gray-300 dark:border-gray-600 pl-4 py-2 my-2 bg-gray-50 dark:bg-gray-800 italic text-gray-700 dark:text-gray-300">${processNestedMarkdown(content)}</blockquote>`;
    })
    // Process lists (both ordered and unordered with nesting)
    .replace(/^((?:[ \t]*(?:\d+\.|\*|\+|\-)\s+.+(?:\n|$))+)/gm, (match) => {
      return processLists(match);
    })
    // Horizontal rules (--- or ***)
    .replace(/^---\s*$/gm, '<hr class="border-t border-gray-300 dark:border-gray-600 my-4">')
    .replace(/^\*\*\*$/gm, '<hr class="border-t border-gray-300 dark:border-gray-600 my-4">')
    // Headings (# ## ### #### ##### ######)
    .replace(/^######\s+(.+)$/gm, '<h6 class="text-sm font-semibold text-gray-900 dark:text-gray-100 mt-4 mb-2">$1</h6>')
    .replace(/^#####\s+(.+)$/gm, '<h5 class="text-base font-semibold text-gray-900 dark:text-gray-100 mt-4 mb-2">$1</h5>')
    .replace(/^####\s+(.+)$/gm, '<h4 class="text-lg font-semibold text-gray-900 dark:text-gray-100 mt-4 mb-2">$1</h4>')
    .replace(/^###\s+(.+)$/gm, '<h3 class="text-xl font-semibold text-gray-900 dark:text-gray-100 mt-4 mb-2">$1</h3>')
    .replace(/^##\s+(.+)$/gm, '<h2 class="text-2xl font-semibold text-gray-900 dark:text-gray-100 mt-4 mb-2">$1</h2>')
    .replace(/^#\s+(.+)$/gm, '<h1 class="text-3xl font-bold text-gray-900 dark:text-gray-100 mt-4 mb-2">$1</h1>')
    // Strikethrough (~~text~~)
    .replace(/~~(.*?)~~/g, '<del>$1</del>')
    // Bold (**text** or __text__)
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/__(.*?)__/g, '<strong>$1</strong>')
    // Italic (*text* or _text_)
    .replace(/\*(.*?)\*/g, '<em>$1</em>')
    .replace(/_(.*?)_/g, '<em>$1</em>')
    // Links [text](url)
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" class="text-blue-500 hover:text-blue-700 underline" target="_blank" rel="noopener noreferrer">$1</a>')
    // Line breaks
    .replace(/\n/g, '<br>');
}

// Helper function to process nested markdown (for blockquotes)
function processNestedMarkdown(text) {
  return text
    // Bold (**text** or __text__)
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/__(.*?)__/g, '<strong>$1</strong>')
    // Italic (*text* or _text_)
    .replace(/\*(.*?)\*/g, '<em>$1</em>')
    .replace(/_(.*?)_/g, '<em>$1</em>')
    // Inline code (`code`)
    .replace(/`([^`]+)`/g, '<code class="bg-gray-100 dark:bg-gray-900 dark:border dark:border-gray-700 px-1 py-0.5 rounded text-sm font-mono">$1</code>')
    // Links [text](url)
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" class="text-blue-500 hover:text-blue-700 underline" target="_blank" rel="noopener noreferrer">$1</a>')
    // Line breaks
    .replace(/\n/g, '<br>');
}

// Helper function to process lists with nesting support
function processLists(text) {
  const lines = text.split('\n').filter(line => line.trim());
  const result = [];
  const stack = []; // Will store objects with {type, level}

  for (const line of lines) {
    const match = line.match(/^(\s*)(\d+\.|\*|\+|\-)\s+(.+)$/);
    if (!match) continue;

    const [, indent, marker, content] = match;
    const level = Math.floor(indent.length / 2); // 2 spaces per level
    const isOrdered = /^\d+\./.test(marker);
    const listType = isOrdered ? 'ol' : 'ul';

    // Close lists that are at deeper or equal levels when moving to a shallower level
    // OR when switching list types at the same level
    while (stack.length > 0 &&
           (stack[stack.length - 1].level > level ||
            (stack[stack.length - 1].level === level && stack[stack.length - 1].type !== listType))) {
      const item = stack.pop();
      result.push(`</li></${item.type}>`);
    }

    // If we need to open a new list (either first list or going deeper)
    if (stack.length === 0 || stack[stack.length - 1].level < level) {
      let listClass;
      if (isOrdered) {
        // Use different numbering styles for different nesting levels
        const numberingStyles = ['decimal', 'lower-alpha', 'lower-roman', 'decimal'];
        const styleIndex = level % numberingStyles.length;
        listClass = `space-y-1 my-2 pl-6`;
        result.push(`<${listType} class="${listClass}" style="list-style-type: ${numberingStyles[styleIndex]};">`);
      } else {
        // Use different bullet styles for different nesting levels
        const bulletStyles = ['disc', 'circle', 'square', 'disc'];
        const styleIndex = level % bulletStyles.length;
        listClass = `space-y-1 my-2 pl-6`;
        result.push(`<${listType} class="${listClass}" style="list-style-type: ${bulletStyles[styleIndex]};">`);
      }

      stack.push({ type: listType, level });
    } else if (stack.length > 0 && stack[stack.length - 1].level === level) {
      // Same level, close previous list item
      result.push('</li>');
    }

    // Add the new list item
    const processedContent = processNestedMarkdown(content);
    result.push(`<li>${processedContent}`);
  }

  // Close all remaining open lists
  while (stack.length > 0) {
    const item = stack.pop();
    result.push(`</li></${item.type}>`);
  }

  return result.join('');
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

document.addEventListener('alpine:init', () => {
  Alpine.store('chat', {
    isOpen: Alpine.$persist(false).using(sessionStorage),
    messages: Alpine.$persist([]).using(sessionStorage),

    toggle() {
      this.isOpen = !this.isOpen;
    },

    close() {
      this.isOpen = false;
    },

    addMessage(message) {
      this.messages.push({
        id: Date.now(),
        ...message,
        timestamp: Date.now()
      });

      // Limit history to 200 messages, keeping newest
      if (this.messages.length > 200) {
        this.messages = this.messages.slice(-200);
      }
    },

    clearMessages() {
      this.messages = [];
    }
  });
});

window.chatComponent = function() {
  return {
    get isOpen() {
      return this.$store.chat.isOpen;
    },

    get messages() {
      return this.$store.chat.messages;
    },

    currentMessage: '',
    isLoading: false,
    inputRows: 1,

    formatContent(content) {
      return processMarkdown(content);
    },

    close() {
      this.$store.chat.close();
    },

    prepareMessageHistory() {
      const messageHistory = [];

      for (const msg of this.messages) {
        const historyMsg = {
          role: msg.role,
          content: msg.fragments ? msg.fragments.content.trim() : msg.content.trim(),
          timestamp: msg.timestamp
        };

        // Include tool calls for assistant messages
        if (msg.role === 'assistant' && msg.toolCalls?.length > 0) {
          historyMsg.tool_calls = msg.toolCalls;
        }

        messageHistory.push(historyMsg);

        // Add tool results as separate tool messages for API context
        if (msg.role === 'assistant' && msg.fragments?.toolResults) {
          for (const toolResult of msg.fragments.toolResults) {
            messageHistory.push({
              role: 'tool',
              content: toolResult.result.trim(),
              tool_call_id: toolResult.tool_call_id,
              timestamp: msg.timestamp
            });
          }
        }
      }

      return messageHistory;
    },

    processContentBuffer(buffer, assistantMessage) {
      while (buffer.length > 0) {
        if (!assistantMessage.inThinking && buffer.includes('<think>')) {
          const idx = buffer.indexOf('<think>');
          assistantMessage.fragments.content += buffer.substring(0, idx);
          assistantMessage.inThinking = true;
          buffer = buffer.substring(idx + 7);
        } else if (assistantMessage.inThinking && buffer.includes('</think>')) {
          const idx = buffer.indexOf('</think>');
          assistantMessage.fragments.thinking += buffer.substring(0, idx);
          assistantMessage.inThinking = false;
          buffer = buffer.substring(idx + 8);
        } else {
          const target = assistantMessage.inThinking ? 'thinking' : 'content';
          assistantMessage.fragments[target] += buffer;
          buffer = '';
        }
      }
      return buffer;
    },

    handleStreamEvent(event, assistantMessage, buffer) {
      switch (event.type) {
        case 'content':
          buffer += event.data;
          return this.processContentBuffer(buffer, assistantMessage);

        case 'tool_calls':
          assistantMessage.toolCalls = event.data;
          break;

        case 'tool_result':
          assistantMessage.fragments.toolResults.push(event.data);
          break;

        case 'error':
          assistantMessage.fragments.content = 'Error: ' + event.data.error;
          break;

        case 'done':
          break;
      }
      return buffer;
    },

    async sendMessage() {
      if (!this.currentMessage.trim() || this.isLoading) return;

      const userMessage = this.currentMessage.trim();
      this.currentMessage = '';
      this.inputRows = 1;

      this.$store.chat.addMessage({
        role: 'user',
        content: userMessage
      });

      this.isLoading = true;
      this.scrollToBottom();

      try {
        const messageHistory = this.prepareMessageHistory();

        const response = await fetch('/api/chat/stream', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ messages: messageHistory })
        });

        if (!response.ok) {
          throw new Error('Failed to send message');
        }

        // Add assistant message after successful request
        this.$store.chat.addMessage({
          role: 'assistant',
          inThinking: false,
          toolCalls: [],
          fragments: {
            thinking: '',
            content: '',
            toolResults: []
          }
        });

        await this.processStreamResponse(response);

      } catch (error) {
        console.error('Chat error:', error);
        const lastMessage = this.messages[this.messages.length - 1];
        if (lastMessage?.role === 'assistant') {
          lastMessage.fragments.content = 'Sorry, I encountered an error. Please try again.';
        }
      } finally {
        this.isLoading = false;
        this.focusInput();
      }
    },

    async processStreamResponse(response) {
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      const assistantMessage = this.messages[this.messages.length - 1];
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value);
        const lines = chunk.split('\n');

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;

          const data = line.slice(6);
          if (data === '[DONE]') continue;

          try {
            const event = JSON.parse(data);
            buffer = this.handleStreamEvent(event, assistantMessage, buffer);
          } catch (e) {
            console.error('Error parsing SSE data:', e);
          }
        }

        this.scrollToBottom();
      }
    },

    scrollToBottom() {
      this.$nextTick(() => {
        const container = this.$refs.messagesContainer;
        if (container) {
          container.scrollTop = container.scrollHeight;
        }
      });
    },

    focusInput() {
      this.$nextTick(() => {
        this.$refs.messageInput?.focus();
      });
    },

    adjustInputSize() {
      this.$nextTick(() => {
        const input = this.$refs.messageInput;
        if (!input) return;

        const style = getComputedStyle(input);
        const lineHeight = parseInt(style.lineHeight);
        const padding = parseInt(style.paddingTop) + parseInt(style.paddingBottom);

        const savedRows = input.rows;
        input.rows = 1;
        input.style.height = 'auto';
        const scrollHeight = input.scrollHeight;
        const neededRows = Math.min(Math.max(1, Math.ceil((scrollHeight - padding) / lineHeight)), 8);
        input.rows = savedRows;
        this.inputRows = neededRows;
        input.style.height = '';
      });
    },

    handleKeyDown(event) {
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        this.sendMessage();
      }
    },

    init() {
      this.$watch('isOpen', (isOpen) => {
        if (isOpen) {
          setTimeout(() => {
            this.scrollToBottom();
            this.focusInput();
          }, 100);
        }
      });

      // Scroll to bottom when component initializes and has messages
      if (this.isOpen && this.messages.length > 0) {
        setTimeout(() => this.scrollToBottom(), 200);
      }
    }
  };
};
