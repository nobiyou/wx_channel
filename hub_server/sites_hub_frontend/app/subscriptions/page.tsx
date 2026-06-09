import { AppShell } from "@/components/shell";
import { SubscriptionConsole } from "@/components/subscription-console";
import { HubUnauthorizedError, getSubscriptions } from "@/lib/hub";
import { requireHubToken } from "@/lib/session";
import { redirect } from "next/navigation";

export default async function SubscriptionsPage() {
  const token = await requireHubToken();
  const subscriptions = await getSubscriptions(token).catch((error) => {
    if (error instanceof HubUnauthorizedError) {
      redirect("/login");
    }
    return [];
  });

  return (
    <AppShell
      title="订阅管理"
      subtitle="这里直接接入 Hub 的订阅操作能力，支持单个更新、全部更新、删除订阅以及查看已抓取视频。"
    >
      <SubscriptionConsole initialSubscriptions={subscriptions} />
    </AppShell>
  );
}
