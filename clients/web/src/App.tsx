import { Suspense, lazy } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { Loader2 } from 'lucide-react';
import { CustomThemeProvider } from '@/theme/ThemeContext';
import { MessageProvider } from '@/context/MessageContext';
import EditorPage from '@/pages/Editor/EditorPage';

const ChatPage = lazy(() => import('@/pages/Chat/ChatPage'));

function RouteFallback() {
  return (
    <div className="w-screen h-screen flex items-center justify-center bg-background">
      <Loader2 className="w-5 h-5 text-muted-foreground animate-spin" />
    </div>
  );
}

function App() {
  return (
    <CustomThemeProvider>
      <MessageProvider>
        <Suspense fallback={<RouteFallback />}>
          <Routes>
            <Route path="/" element={<EditorPage />} />
            <Route path="/chat" element={<ChatPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </MessageProvider>
    </CustomThemeProvider>
  );
}

export default App;
