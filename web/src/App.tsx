import { NavLink, Route, Routes } from "react-router";

import { LaunchPage } from "./pages/LaunchPage";
import { WorkloadDetailPage } from "./pages/WorkloadDetailPage";
import { WorkloadsPage } from "./pages/WorkloadsPage";

export function App() {
  return (
    <div className="shell">
      <header className="topbar">
        <span className="brand">Tensora</span>
        <nav>
          <NavLink to="/" end>
            Workloads
          </NavLink>
          <NavLink to="/launch">Launch</NavLink>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<WorkloadsPage />} />
          <Route path="/launch" element={<LaunchPage />} />
          <Route path="/workloads/:id" element={<WorkloadDetailPage />} />
        </Routes>
      </main>
    </div>
  );
}
