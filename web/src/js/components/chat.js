import Alpine from 'alpinejs';

document.addEventListener('alpine:init', () => {
  Alpine.store('chat', {
    isOpen: Alpine.$persist(false),
    messages: Alpine.$persist([]),

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
        content: '',
        thinking: '',
        inThinking: false,
        toolResults: []
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
                      assistantMessage.content += buffer.substring(0, idx);
                      assistantMessage.inThinking = true;
                      buffer = buffer.substring(idx + 7);
                    } else if (assistantMessage.inThinking && buffer.includes('</think>')) {
                      const idx = buffer.indexOf('</think>');
                      assistantMessage.thinking += buffer.substring(0, idx);
                      assistantMessage.inThinking = false;
                      buffer = buffer.substring(idx + 8);
                    } else {
                      if (assistantMessage.inThinking) {
                        assistantMessage.thinking += buffer;
                      } else {
                        assistantMessage.content += buffer;
                      }
                      buffer = '';
                    }
                  }
                } else if (event.type === 'tool_result') {
                  if (!assistantMessage.toolResults) {
                    assistantMessage.toolResults = [];
                  }
                  assistantMessage.toolResults.push(event.data);
                } else if (event.type === 'error') {
                  assistantMessage.content = 'Error: ' + event.data.error;
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
          assistantMessage.content = 'Sorry, I encountered an error. Please try again.';
        }
      } finally {
        this.isLoading = false;
        this.focusInput();
      }
    },

    formatMessage(content) {
      return content
        .replace(/<think>[\s\S]*?<\/think>/g, '')
        .replace(/^\s+/, '')
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.*?)\*/g, '<em>$1</em>')
        .replace(/`(.*?)`/g, '<code class="bg-gray-200 dark:bg-gray-600 px-1 rounded">$1</code>')
        .replace(/\n/g, '<br>')
        .replace(/^(<br>\s*)+/, '');
    },

    extractThinking(content) {
      const thinkRegex = /<think>([\s\S]*?)<\/think>/g;
      const match = thinkRegex.exec(content);
      return match ? match[1].trim() : null;
    },

    removeThinking(content) {
      return content.replace(/<think>[\s\S]*?<\/think>/g, '').trim();
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