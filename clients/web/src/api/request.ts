import { useMessageStore } from '@/store/useAppStore';

const BASE_URL = import.meta.env.VITE_API_BASE_URL || '';

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const url = `${BASE_URL}${path}`;
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  try {
    const response = await fetch(url, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: response.statusText }));
      const msg = error.message || `HTTP Error: ${response.status}`;
      useMessageStore.getState().show(msg, 'error');
      throw new Error(msg);
    }

    return await response.json();
  } catch (error) {
    useMessageStore.getState().show('Network connection failure', 'error');
    throw error;
  }
}

const client = {
  get: <T>(url: string, options?: RequestInit) => 
    request<T>(url, { ...options, method: 'GET' }),
  
  post: <T>(url: string, data?: unknown, options?: RequestInit) => 
    request<T>(url, { 
      ...options, 
      method: 'POST', 
      body: data ? JSON.stringify(data) : undefined 
    }),
  
  put: <T>(url: string, data?: unknown, options?: RequestInit) => 
    request<T>(url, { 
      ...options, 
      method: 'PUT', 
      body: data ? JSON.stringify(data) : undefined 
    }),
  
  delete: <T>(url: string, options?: RequestInit) => 
    request<T>(url, { ...options, method: 'DELETE' }),
};

export default client;
