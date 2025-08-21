import Alpine from 'alpinejs';

// Simple markdown processor for chat messages
function processMarkdown(text) {
  if (!text) return '';

  // First, let's protect code blocks by replacing them with placeholders
  const codeBlocks = [];
  const inlineCodeBlocks = [];

  // Extract and protect fenced code blocks first
  text = text.replace(/```[\s\S]*?```/g, (match) => {
    const placeholder = `CODEBLOCK${codeBlocks.length}PLACEHOLDER`;
    codeBlocks.push(match);
    return placeholder;
  });

  // Extract and protect inline code blocks
  text = text.replace(/`[^`\n]+`/g, (match) => {
    const placeholder = `INLINECODE${inlineCodeBlocks.length}PLACEHOLDER`;
    inlineCodeBlocks.push(match);
    return placeholder;
  });

  // Process tables
  text = text.replace(/^((?:\|.*\|\s*\n)+)/gm, (match) => {
    return processTable(match);
  });

  // Now process all other markdown
  text = text
    .trim()
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
    .replace(/\b\*((?:[^*\s](?:[^*]*[^*\s])?)*)\*\b/g, '<em>$1</em>')
    .replace(/\b_((?:[^_\s](?:[^_]*[^_\s])?)*?)_\b/g, '<em>$1</em>')
    // Links [text](url)
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" class="text-blue-500 hover:text-blue-700 underline" target="_blank" rel="noopener noreferrer">$1</a>')
    // Line breaks
    .replace(/\n/g, '<br>');

  // Restore inline code blocks
  inlineCodeBlocks.forEach((code, index) => {
    const placeholder = `INLINECODE${index}PLACEHOLDER`;
    const codeContent = code.slice(1, -1); // Remove backticks
    const replacement = `<code class="bg-gray-100 dark:bg-gray-900 dark:border dark:border-gray-700 px-1 py-0.5 rounded text-sm font-mono">${escapeHtml(codeContent)}</code>`;
    text = text.replace(placeholder, replacement);
  });

  // Restore fenced code blocks
  codeBlocks.forEach((code, index) => {
    const placeholder = `CODEBLOCK${index}PLACEHOLDER`;
    const match = code.match(/```(\w+)?\s*([\s\S]*?)\s*```/);
    if (match) {
      const [, lang, codeContent] = match;
      const language = lang || 'text';
      const replacement = `<pre class="bg-gray-100 dark:bg-gray-900 dark:border dark:border-gray-700 p-3 rounded-md overflow-x-auto my-2"><code class="language-${language} text-sm">${escapeHtml(codeContent.trim())}</code></pre>`;
      text = text.replace(placeholder, replacement);
    }
  });

  return text;
}

// Helper function to process nested markdown (for blockquotes)
function processNestedMarkdown(text) {
  return text
    // Bold (**text** or __text__)
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/__(.*?)__/g, '<strong>$1</strong>')
    // Italic (*text* or _text_)
    .replace(/\b\*((?:[^*\s](?:[^*]*[^*\s])?)*)\*\b/g, '<em>$1</em>')
    .replace(/\b_((?:[^_\s](?:[^_]*[^_\s])?)*?)_\b/g, '<em>$1</em>')
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

// Helper function to process tables
function processTable(text) {
  const lines = text.trim().split('\n').filter(line => line.trim());
  if (lines.length < 2) return text;

  const tableRows = [];
  let hasHeader = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    if (!line.startsWith('|') || !line.endsWith('|')) continue;

    const content = line.slice(1, -1);

    // Check if this is a separator line (only dashes, colons, spaces, and pipes)
    if (/^[\s\-:|]+$/.test(content) && content.includes('-')) {
      // This is a separator - mark that we have a header and skip this line
      if (tableRows.length === 1) {
        hasHeader = true;
      }
      continue;
    }

    // Parse table cells
    const cells = content.split('|').map(cell => cell.trim());
    tableRows.push(cells);
  }

  if (tableRows.length === 0) return text;

  let html = '<div class="overflow-x-auto my-4"><table class="min-w-full border-collapse border border-gray-300 dark:border-gray-600">';

  if (hasHeader && tableRows.length > 0) {
    // First row is header
    html += '<thead class="bg-gray-50 dark:bg-gray-800"><tr>';
    for (const cell of tableRows[0]) {
      html += `<th class="border border-gray-300 dark:border-gray-600 px-3 py-2 text-left font-semibold">${processNestedMarkdown(cell)}</th>`;
    }
    html += '</tr></thead>';

    // Remaining rows are body
    if (tableRows.length > 1) {
      html += '<tbody>';
      for (let i = 1; i < tableRows.length; i++) {
        html += '<tr class="even:bg-gray-50 dark:even:bg-gray-800">';
        for (const cell of tableRows[i]) {
          html += `<td class="border border-gray-300 dark:border-gray-600 px-3 py-2">${processNestedMarkdown(cell)}</td>`;
        }
        html += '</tr>';
      }
      html += '</tbody>';
    }
  } else {
    // No header, all rows are body
    html += '<tbody>';
    for (const row of tableRows) {
      html += '<tr class="even:bg-gray-50 dark:even:bg-gray-800">';
      for (const cell of row) {
        html += `<td class="border border-gray-300 dark:border-gray-600 px-3 py-2">${processNestedMarkdown(cell)}</td>`;
      }
      html += '</tr>';
    }
    html += '</tbody>';
  }

  html += '</table></div>';
  return html;
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
    // Separate persistent storage for input history - independent of conversation
    inputHistory: Alpine.$persist([]).using(sessionStorage),

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

      // Limit history to 50 messages, keeping newest
      if (this.messages.length > 50) {
        this.messages = this.messages.slice(-50);
      }
    },

    addToInputHistory(message) {
      // Remove duplicate if it exists
      const index = this.inputHistory.indexOf(message);
      if (index > -1) {
        this.inputHistory.splice(index, 1);
      }

      // Add to end of array
      this.inputHistory.push(message);

      // Keep only last 50 entries
      if (this.inputHistory.length > 50) {
        this.inputHistory = this.inputHistory.slice(-50);
      }
    },

    clearMessages() {
      this.messages = [];
      // Note: We deliberately do NOT clear inputHistory here
    }
  });
});

window.chatComponent = function () {
  return {
    get isOpen() {
      return this.$store.chat.isOpen;
    },

    get messages() {
      return this.$store.chat.messages;
    },

    get inputHistory() {
      return this.$store.chat.inputHistory;
    },

    currentMessage: '',
    isLoading: false,
    inputRows: 1,
    historyIndex: -1,
    partialMessage: '',
    abortController: null,

    formatContent(content) {
      return processMarkdown(content);
    },

    close() {
      this.$store.chat.close();
    },

    prepareMessageHistory() {
      const messageHistory = [];

      for (const msg of this.messages) {
        let content = msg.fragments ? msg.fragments.content.trim() : msg.content.trim();

        // Strip any think tags that might exist in the content to prevent LLM template errors
        content = content.replace(/<think>[\s\S]*?<\/think>/g, '').trim();

        const historyMsg = {
          role: msg.role,
          content: content,
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
          assistantMessage.fragments.content = '⚠️ ' + (event.data.error || 'An error occurred while processing your request.');
          assistantMessage.hasError = true;
          break;

        case 'done':
          break;
      }
      return buffer;
    },

    async sendMessage() {
      if (!this.currentMessage.trim() || this.isLoading) return;

      const userMessage = this.currentMessage.trim();

      // Add to persistent input history (independent of conversation)
      this.$store.chat.addToInputHistory(userMessage);

      this.currentMessage = '';
      this.inputRows = 1;
      this.historyIndex = -1;
      this.partialMessage = '';

      this.$store.chat.addMessage({
        role: 'user',
        content: userMessage
      });

      this.isLoading = true;
      this.abortController = new AbortController();
      this.scrollToBottom();

      try {
        const messageHistory = this.prepareMessageHistory();

        const response = await fetch('/api/chat/stream', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ messages: messageHistory }),
          signal: this.abortController.signal
        });

        if (!response.ok) {
          let errorMessage = 'Failed to send message';
          try {
            const errorData = await response.json();
            errorMessage = errorData.error || errorMessage;
          } catch (e) {
            // If we can't parse the error response, use the status text
            errorMessage = response.statusText || errorMessage;
          }
          throw new Error(errorMessage);
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
        if (error.name === 'AbortError') {
          const lastMessage = this.messages[this.messages.length - 1];
          if (lastMessage?.role === 'assistant') {
            lastMessage.fragments.content += '\n\n*[Response stopped by user]*';
          }
        } else {
          const lastMessage = this.messages[this.messages.length - 1];
          if (lastMessage?.role === 'assistant') {
            lastMessage.fragments.content = '⚠️ Failed to connect to the AI service. Please check your connection and try again.';
            lastMessage.hasError = true;
          } else {
            // If no assistant message exists, create one with the error
            this.$store.chat.addMessage({
              role: 'assistant',
              inThinking: false,
              toolCalls: [],
              hasError: true,
              fragments: {
                thinking: '',
                content: '⚠️ Failed to connect to the AI service. Please check your connection and try again.',
                toolResults: []
              }
            });
          }
        }
      } finally {
        this.isLoading = false;
        this.abortController = null;
        this.focusInput();
      }
    },

    stopGeneration() {
      if (this.abortController) {
        this.abortController.abort();
      }
    },

    async processStreamResponse(response) {
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      const assistantMessage = this.messages[this.messages.length - 1];
      let buffer = '';
      let lastActivityTime = Date.now();
      const TIMEOUT_MS = 300000; // 5 minute timeout

      try {
        while (true) {
          // Check for timeout
          if (Date.now() - lastActivityTime > TIMEOUT_MS) {
            throw new Error('Stream timeout - no data received for 5 minutes');
          }

          const { done, value } = await reader.read();
          if (done) break;

          lastActivityTime = Date.now(); // Reset timeout on activity
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
              // If we can't parse the event, it might be a malformed response
              // Add this to the assistant message as an error
              if (assistantMessage && !assistantMessage.hasError) {
                assistantMessage.fragments.content += '\n\n⚠️ Received malformed response from AI service.';
                assistantMessage.hasError = true;
              }
            }
          }

          this.scrollToBottom();
        }
      } catch (error) {
        if (assistantMessage && !assistantMessage.hasError) {
          assistantMessage.fragments.content += '\n\n⚠️ Stream processing failed: ' + error.message;
          assistantMessage.hasError = true;
        }
        throw error;
      } finally {
        try {
          reader.releaseLock();
        } catch (e) {
          // Ignore lock release errors
        }
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
      } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        this.navigateHistory('up');
      } else if (event.key === 'ArrowDown') {
        event.preventDefault();
        this.navigateHistory('down');
      }
    },

    navigateHistory(direction) {
      if (this.inputHistory.length === 0) return;

      if (direction === 'up') {
        if (this.historyIndex === -1) {
          // Save current partial message
          this.partialMessage = this.currentMessage;
          this.historyIndex = this.inputHistory.length - 1;
        } else if (this.historyIndex > 0) {
          this.historyIndex--;
        }
        this.currentMessage = this.inputHistory[this.historyIndex];
      } else if (direction === 'down') {
        if (this.historyIndex === -1) return;

        if (this.historyIndex < this.inputHistory.length - 1) {
          this.historyIndex++;
          this.currentMessage = this.inputHistory[this.historyIndex];
        } else {
          // Return to partial message or empty
          this.historyIndex = -1;
          this.currentMessage = this.partialMessage;
        }
      }

      this.adjustInputSize();
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
