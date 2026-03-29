import { useMessageStore } from '@/store/useAppStore';

export const useMessage = () => {
  const { show, showError, showSuccess, hide } = useMessageStore();
  return {
    showMessage: show,
    showError,
    showSuccess,
    showWarning: (msg: string) => show(msg, 'warning'),
    hide
  };
};
