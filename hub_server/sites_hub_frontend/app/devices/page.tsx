import { AppShell } from "@/components/shell";
import { DeviceConsole } from "@/components/device-console";
import { HubUnauthorizedError, getUserDevices } from "@/lib/hub";
import { requireHubToken } from "@/lib/session";
import { redirect } from "next/navigation";

export default async function DevicesPage() {
  const token = await requireHubToken();
  const devices = await getUserDevices(token).catch((error) => {
    if (error instanceof HubUnauthorizedError) {
      redirect("/login");
    }
    return [];
  });

  return (
    <AppShell
      title="设备管理"
      subtitle="这里已经接上 Hub 的真实设备管理链路：生成绑定码、刷新归属设备、改名、分组、锁定、解绑和删除都走站点内代理。"
    >
      <DeviceConsole initialDevices={devices} />
    </AppShell>
  );
}
