import { Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "./auth";
import { ChatPage } from "./pages/Chat";
import { LoginPage } from "./pages/Login";
import { SettingsPage } from "./pages/Settings";

function Guard({ children }: { children: React.ReactNode }) {
  const { state } = useAuth();
  if (!state.access) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

export function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route
          path="/"
          element={
            <Guard>
              <ChatPage />
            </Guard>
          }
        />
        <Route
          path="/settings"
          element={
            <Guard>
              <SettingsPage />
            </Guard>
          }
        />
      </Routes>
    </AuthProvider>
  );
}
