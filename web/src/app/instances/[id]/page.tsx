import InstanceDetailPage from './instance-detail';

export async function generateStaticParams() {
  return [{ id: '_' }];
}

export default function Page() {
  return <InstanceDetailPage />;
}
