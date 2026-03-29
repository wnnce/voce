import React from 'react';
import { Snackbar, Alert } from '@mui/material';
import { useMessageStore } from '@/store/useAppStore';

export const MessageProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { message, severity, open, hide } = useMessageStore();

  return (
    <>
      {children}
      <Snackbar 
        open={open} 
        autoHideDuration={3000} 
        onClose={hide}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
        sx={{ mt: 7, zIndex: 2000 }} // Ensure it's above Topbar (1600)
      >
        <Alert 
          onClose={hide} 
          severity={severity} 
          variant="filled" 
          sx={{ width: '100%', borderRadius: '10px', boxShadow: '0 4px 20px rgba(0,0,0,0.15)' }}
        >
          {message}
        </Alert>
      </Snackbar>
    </>
  );
};

// Note: useMessage hook has been moved to '@/hooks/useMessage'
