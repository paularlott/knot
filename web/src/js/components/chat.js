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
    
    close() {
      this.$store.chat.close();
    },
    
    async sendMessage() {
      if (!this.currentMessage.trim() || this.isLoading) return;
      
      const userMessage = this.currentMessage.trim();
      this.currentMessage = '';
      
      this.$store.chat.addMessage({
        role: 'user',
        content: userMessage
      });
      
      const assistantMessageId = Date.now() + 1;
      this.$store.chat.addMessage({
        id: assistantMessageId,
        role: 'assistant',
        content: '',
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
                  assistantMessage.content += event.data;
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
      }
    },
    
    formatMessage(content) {
      return content
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.*?)\*/g, '<em>$1</em>')
        .replace(/`(.*?)`/g, '<code class="bg-gray-200 dark:bg-gray-600 px-1 rounded">$1</code>')
        .replace(/\n/g, '<br>');
    },
    
    scrollToBottom() {
      this.$nextTick(() => {
        const container = this.$refs.messagesContainer;
        if (container) {
          container.scrollTop = container.scrollHeight;
        }
      });
    },
    
    init() {
      this.$watch('isOpen', (isOpen) => {
        if (isOpen) {
          setTimeout(() => {
            this.scrollToBottom();
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