import Alpine from 'alpinejs';

// Simple markdown processor for chat messages
function processMarkdown(text) {
  if (!text) return '';

  return text
    .trim() // Remove leading/trailing whitespace
    // Code blocks (```language\ncode\n```)
    .replace(/```(\w+)?\n([\s\S]*?)\n```/g, (match, lang, code) => {
      const language = lang || 'text';
      return `<pre class="bg-gray-100 dark:bg-gray-800 p-3 rounded-md overflow-x-auto my-2"><code class="language-${language} text-sm">${escapeHtml(code.trim())}</code></pre>`;
    })
    // Inline code (`code`)
    .replace(/`([^`]+)`/g, '<code class="bg-gray-100 dark:bg-gray-800 px-1 py-0.5 rounded text-sm font-mono">$1</code>')
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

    // Markdown processing function
    formatContent(content) {
      return processMarkdown(content);
    },

    close() {
      this.$store.chat.close();
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

      const assistantMessageId = Date.now() + 1;
      this.$store.chat.addMessage({
        id: assistantMessageId,
        role: 'assistant',
        inThinking: false,
        toolResults: [],
        // Add structured content storage
        fragments: {
          thinking: '',
          content: '',
          toolResults: []
        }
      });

      this.isLoading = true;
      this.scrollToBottom();

      try {
        const response = await fetch('/api/chat/stream', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            message: userMessage
          })
        });

        if (!response.ok) {
          throw new Error('Failed to send message');
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let assistantMessage = this.messages.find(m => m.id === assistantMessageId);
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          const chunk = decoder.decode(value);
          const lines = chunk.split('\n');

          for (const line of lines) {
            if (line.startsWith('data: ')) {
              const data = line.slice(6);
              if (data === '[DONE]') continue;

              try {
                const event = JSON.parse(data);

                if (event.type === 'content') {
                  buffer += event.data;

                  while (buffer.length > 0) {
                    if (!assistantMessage.inThinking && buffer.includes('<think>')) {
                      const idx = buffer.indexOf('<think>');
                      const contentPart = buffer.substring(0, idx);
                      assistantMessage.fragments.content += contentPart;
                      assistantMessage.inThinking = true;
                      buffer = buffer.substring(idx + 7);
                    } else if (assistantMessage.inThinking && buffer.includes('</think>')) {
                      const idx = buffer.indexOf('</think>');
                      const thinkingPart = buffer.substring(0, idx);
                      assistantMessage.fragments.thinking += thinkingPart;
                      assistantMessage.inThinking = false;
                      buffer = buffer.substring(idx + 8);
                    } else {
                      if (assistantMessage.inThinking) {
                        assistantMessage.fragments.thinking += buffer;
                      } else {
                        assistantMessage.fragments.content += buffer;
                      }
                      buffer = '';
                    }
                  }
                } else if (event.type === 'tool_result') {
                  if (!assistantMessage.toolResults) {
                    assistantMessage.toolResults = [];
                  }
                  assistantMessage.toolResults.push(event.data);
                  assistantMessage.fragments.toolResults.push(event.data);
                } else if (event.type === 'error') {
                  assistantMessage.fragments.content = 'Error: ' + event.data.error;
                } else if (event.type === 'done') {
                  break;
                }
              } catch (e) {
                console.error('Error parsing SSE data:', e);
              }
            }
          }

          this.scrollToBottom();
        }
      } catch (error) {
        console.error('Chat error:', error);
        const assistantMessage = this.messages.find(m => m.id === assistantMessageId);
        if (assistantMessage) {
          assistantMessage.fragments.content = 'Sorry, I encountered an error. Please try again.';
        }
      } finally {
        this.isLoading = false;
        this.focusInput();
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
        const input = this.$refs.messageInput;
        if (input) {
          input.focus();
        }
      });
    },

    adjustInputSize() {
      this.$nextTick(() => {
        const input = this.$refs.messageInput;
        if (input) {
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
        }
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
        setTimeout(() => {
          this.scrollToBottom();
        }, 200);
      }
    }
  };
};